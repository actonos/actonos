package mailcal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/google/uuid"
)

// Account kinds.
const (
	KindMail     = "mail"
	KindCalendar = "calendar"
	KindBoth     = "both"
)

// Account statuses. StatusPaused is used from Phase 2 onwards by the policy engine.
const (
	StatusOK          = "ok"
	StatusAuthExpired = "auth_expired"
	StatusPaused      = "paused"
	StatusError       = "error"
)

const maxConfigBytes = 16 * 1024

var (
	// ErrAccountNotFound is returned for an unknown account id.
	ErrAccountNotFound = errors.New("mail/calendar account not found")
	// ErrInvalidAccount is returned when account input fails validation.
	ErrInvalidAccount = errors.New("invalid mail/calendar account")
)

// AuthRef says how an account authenticates without holding any secret itself. Secrets live
// in the vault (or the OAuth token store) and are referenced by name.
type AuthRef struct {
	// Type is "none", "oauth" or "password".
	Type string `json:"type"`
	// Provider names the OAuth provider key (Phase 3), e.g. "mailcal:<account id>".
	Provider string `json:"provider,omitempty"`
	// SecretRefs maps a role ("password", "client_id", "client_secret") to a vault key.
	SecretRefs map[string]string `json:"secret_refs,omitempty"`
}

// Account is one mailbox and/or calendar served by a provider plugin.
type Account struct {
	ID           string         `json:"id"`
	Label        string         `json:"label"`
	PluginID     string         `json:"plugin_id"`
	Kind         string         `json:"kind"`
	Email        string         `json:"email,omitempty"` // the owner's own normalized address
	Config       map[string]any `json:"config,omitempty"`
	Auth         AuthRef        `json:"auth"`
	Status       string         `json:"status"`
	StatusDetail string         `json:"status_detail,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// Ref converts the account to the ABI reference passed to providers.
func (a Account) Ref() abi.AccountRef { return abi.AccountRef{ID: a.ID, Config: a.Config} }

// SupportsMail reports whether the account can serve mail operations.
func (a Account) SupportsMail() bool { return a.Kind == KindMail || a.Kind == KindBoth }

// SupportsCalendar reports whether the account can serve calendar operations.
func (a Account) SupportsCalendar() bool { return a.Kind == KindCalendar || a.Kind == KindBoth }

// NewAccount is the input to AccountStore.Create.
type NewAccount struct {
	Label    string
	PluginID string
	Kind     string
	Email    string
	Config   map[string]any
	Auth     AuthRef
}

// AccountStore persists accounts in SQLite.
type AccountStore struct{ db *sql.DB }

// NewAccountStore creates the table if needed.
func NewAccountStore(db *sql.DB) (*AccountStore, error) {
	if db == nil {
		return nil, errors.New("mailcal: database is required")
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS mailcal_accounts (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			plugin_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			config_json TEXT NOT NULL DEFAULT '{}',
			auth_json TEXT NOT NULL DEFAULT '{"type":"none"}',
			status TEXT NOT NULL DEFAULT 'ok',
			status_detail TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`)
	if err != nil {
		return nil, fmt.Errorf("creating mailcal_accounts: %w", err)
	}
	return &AccountStore{db: db}, nil
}

func validateKind(kind string) error {
	switch kind {
	case KindMail, KindCalendar, KindBoth:
		return nil
	}
	return fmt.Errorf("%w: kind must be mail, calendar or both", ErrInvalidAccount)
}

// Create validates and stores a new account.
func (s *AccountStore) Create(ctx context.Context, in NewAccount) (Account, error) {
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" {
		return Account{}, fmt.Errorf("%w: label is required", ErrInvalidAccount)
	}
	if strings.TrimSpace(in.PluginID) == "" {
		return Account{}, fmt.Errorf("%w: plugin_id is required", ErrInvalidAccount)
	}
	if err := validateKind(in.Kind); err != nil {
		return Account{}, err
	}
	email := ""
	if strings.TrimSpace(in.Email) != "" {
		var err error
		if email, err = abi.NormalizeEmail(in.Email); err != nil {
			return Account{}, fmt.Errorf("%w: email: %v", ErrInvalidAccount, err)
		}
	}
	if in.Auth.Type == "" {
		in.Auth.Type = "none"
	}
	switch in.Auth.Type {
	case "none", "oauth", "password":
	default:
		return Account{}, fmt.Errorf("%w: auth.type must be none, oauth or password", ErrInvalidAccount)
	}
	if err := rejectSecrets(in.Config); err != nil {
		return Account{}, err
	}
	cfg, err := json.Marshal(nonNilMap(in.Config))
	if err != nil || len(cfg) > maxConfigBytes {
		return Account{}, fmt.Errorf("%w: config must be JSON under %d bytes", ErrInvalidAccount, maxConfigBytes)
	}
	auth, _ := json.Marshal(in.Auth)

	now := time.Now().UTC()
	acct := Account{
		ID: "acct_" + uuid.NewString(), Label: in.Label, PluginID: in.PluginID, Kind: in.Kind, Email: email,
		Config: nonNilMap(in.Config), Auth: in.Auth, Status: StatusOK, CreatedAt: now, UpdatedAt: now,
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO mailcal_accounts (id, label, plugin_id, kind, email, config_json, auth_json, status, status_detail, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		acct.ID, acct.Label, acct.PluginID, acct.Kind, acct.Email, string(cfg), string(auth), acct.Status, now, now)
	if err != nil {
		return Account{}, fmt.Errorf("storing account: %w", err)
	}
	return acct, nil
}

var secretKeyRE = regexp.MustCompile(`(?i)(pass(word|wd)?|secret|token|api[_-]?key|credential|private[_-]?key)`)

// rejectSecrets enforces the contract that Config is non-secret: anything that looks like a
// credential belongs in the vault and is referenced from Auth.SecretRefs.
func rejectSecrets(config map[string]any) error {
	var walk func(prefix string, v any) error
	walk = func(prefix string, v any) error {
		switch t := v.(type) {
		case map[string]any:
			for k, child := range t {
				path := k
				if prefix != "" {
					path = prefix + "." + k
				}
				if secretKeyRE.MatchString(k) {
					return fmt.Errorf("%w: config key %q looks like a secret; store it in the vault and reference it from auth.secret_refs", ErrInvalidAccount, path)
				}
				if err := walk(path, child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range t {
				if err := walk(prefix, child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk("", config)
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

const accountColumns = `id, label, plugin_id, kind, email, config_json, auth_json, status, status_detail, created_at, updated_at`

func scanAccount(row interface{ Scan(...any) error }) (Account, error) {
	var a Account
	var cfg, auth string
	if err := row.Scan(&a.ID, &a.Label, &a.PluginID, &a.Kind, &a.Email, &cfg, &auth, &a.Status, &a.StatusDetail, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return Account{}, err
	}
	if err := json.Unmarshal([]byte(cfg), &a.Config); err != nil {
		return Account{}, fmt.Errorf("decoding config of %s: %w", a.ID, err)
	}
	if err := json.Unmarshal([]byte(auth), &a.Auth); err != nil {
		return Account{}, fmt.Errorf("decoding auth of %s: %w", a.ID, err)
	}
	return a, nil
}

// Get returns one account.
func (s *AccountStore) Get(ctx context.Context, id string) (Account, error) {
	a, err := scanAccount(s.db.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM mailcal_accounts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	return a, err
}

// List returns every account, oldest first.
func (s *AccountStore) List(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+accountColumns+` FROM mailcal_accounts ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Update changes the label, owner email and non-secret config.
func (s *AccountStore) Update(ctx context.Context, id, label, email string, config map[string]any) (Account, error) {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return Account{}, err
	}
	if label = strings.TrimSpace(label); label == "" {
		return Account{}, fmt.Errorf("%w: label is required", ErrInvalidAccount)
	}
	normalized := ""
	if strings.TrimSpace(email) != "" {
		if normalized, err = abi.NormalizeEmail(email); err != nil {
			return Account{}, fmt.Errorf("%w: email: %v", ErrInvalidAccount, err)
		}
	}
	if err := rejectSecrets(config); err != nil {
		return Account{}, err
	}
	cfg, err := json.Marshal(nonNilMap(config))
	if err != nil || len(cfg) > maxConfigBytes {
		return Account{}, fmt.Errorf("%w: config must be JSON under %d bytes", ErrInvalidAccount, maxConfigBytes)
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `UPDATE mailcal_accounts SET label=?, email=?, config_json=?, updated_at=? WHERE id=?`,
		label, normalized, string(cfg), now, id); err != nil {
		return Account{}, err
	}
	cur.Label, cur.Email, cur.Config, cur.UpdatedAt = label, normalized, nonNilMap(config), now
	return cur, nil
}

// SetAuth replaces how the account authenticates (used by OAuth and password setup).
func (s *AccountStore) SetAuth(ctx context.Context, id string, auth AuthRef) error {
	raw, _ := json.Marshal(auth)
	res, err := s.db.ExecContext(ctx, `UPDATE mailcal_accounts SET auth_json=?, updated_at=? WHERE id=?`, string(raw), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAccountNotFound
	}
	return nil
}

// SetStatus records the account health shown in the UI and consulted before every call.
func (s *AccountStore) SetStatus(ctx context.Context, id, status, detail string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE mailcal_accounts SET status=?, status_detail=?, updated_at=? WHERE id=?`,
		status, detail, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAccountNotFound
	}
	return nil
}

// Delete removes an account and its drafts.
func (s *AccountStore) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM mailcal_accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAccountNotFound
	}
	// Drafts may not exist yet if the draft store was never created; ignore that case only.
	if _, err := tx.ExecContext(ctx, `DELETE FROM mailcal_drafts WHERE account_id = ?`, id); err != nil && !strings.Contains(err.Error(), "no such table") {
		return err
	}
	return tx.Commit()
}
