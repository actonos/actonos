package mailcal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/certificate"
	"github.com/actonos/actonos/internal/plugin"
	"github.com/actonos/actonos/internal/tools"
)

const probeAccountID = "_probe"

// ProviderInfo describes a loaded provider plugin.
type ProviderInfo struct {
	PluginID string     `json:"plugin_id"`
	Name     string     `json:"name"`
	Version  string     `json:"version"`
	ABI      []string   `json:"abi"`
	Ops      []string   `json:"ops"`
	Limits   abi.Limits `json:"limits"`
	// OAuthProvider is set when the plugin signs users in through ActonOS (for example "google").
	OAuthProvider string `json:"oauth_provider,omitempty"`
	// AuthKind tells the UI which sign-in form to show: "oauth" (the core runs the flow),
	// "password" (the plugin receives the account's login through acton_account) or "".
	AuthKind string `json:"auth_kind,omitempty"`
	// Verified is true when the plugin directory holds a conformance certificate for exactly
	// this plugin.wasm. It is information for the person installing the plugin; every plugin
	// runs in the same sandbox and behind the same gates whether or not it is verified.
	Verified bool `json:"verified"`
}

type providerEntry struct {
	info     ProviderInfo
	provider Provider
	caps     abi.Capabilities
	oauth    *plugin.PluginOAuth
}

// Service is the core of mail and calendar support. It owns accounts and drafts, knows
// which provider plugins are loaded (via plugin.CapabilityHook), and serves the tools.
type Service struct {
	accounts  *AccountStore
	drafts    *DraftStore
	settings  *SettingsStore
	rules     *RuleStore
	proposals *ProposalStore
	decisions *DecisionStore
	ledger    *Ledger

	mu        sync.RWMutex
	providers map[string]*providerEntry
	registry  *tools.ToolRegistry
	toolNames []string
	// pluginsDir is where installed plugins live (<dir>/<plugin id>/plugin.wasm); empty means
	// no plugin is reported as verified.
	pluginsDir string

	// sleep is replaceable so tests do not wait for real back-off delays.
	sleep func(ctx context.Context, d time.Duration) error
}

var _ plugin.CapabilityHook = (*Service)(nil)

// NewService creates the stores and returns a service with no providers.
func NewService(db *sql.DB) (*Service, error) {
	accounts, err := NewAccountStore(db)
	if err != nil {
		return nil, err
	}
	drafts, err := NewDraftStore(db)
	if err != nil {
		return nil, err
	}
	svc := &Service{accounts: accounts, drafts: drafts, providers: map[string]*providerEntry{}, sleep: sleepCtx}
	if svc.settings, err = NewSettingsStore(db); err != nil {
		return nil, err
	}
	if svc.rules, err = NewRuleStore(db); err != nil {
		return nil, err
	}
	if svc.proposals, err = NewProposalStore(db, svc.rules); err != nil {
		return nil, err
	}
	if svc.decisions, err = NewDecisionStore(db); err != nil {
		return nil, err
	}
	if svc.ledger, err = NewLedger(db); err != nil {
		return nil, err
	}
	return svc, nil
}

// Settings returns the master switch store.
func (s *Service) Settings() *SettingsStore { return s.settings }

// Rules returns the rule book. Hand it only to code that runs on behalf of an authenticated person.
func (s *Service) Rules() *RuleStore { return s.rules }

// Proposals returns the proposal store.
func (s *Service) Proposals() *ProposalStore { return s.proposals }

// Decisions returns the audit trail.
func (s *Service) Decisions() *DecisionStore { return s.decisions }

// Ledger returns the idempotency ledger.
func (s *Service) Ledger() *Ledger { return s.ledger }

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Accounts exposes the account store (used by the REST API).
func (s *Service) Accounts() *AccountStore { return s.accounts }

// Drafts exposes the draft store (used by the REST API and, from Phase 2, the policy engine).
func (s *Service) Drafts() *DraftStore { return s.drafts }

// SetToolRegistry tells the service where to publish its tools. Tools appear only while at
// least one account exists, so agents without email do not see them.
func (s *Service) SetToolRegistry(r *tools.ToolRegistry) {
	s.mu.Lock()
	s.registry = r
	s.mu.Unlock()
}

// SetPluginsDir tells the service where installed plugins live so it can report which of
// them carry a valid conformance certificate. Call it before plugins are scanned.
func (s *Service) SetPluginsDir(dir string) {
	s.mu.Lock()
	s.pluginsDir = dir
	s.mu.Unlock()
}

// PluginActivated implements plugin.CapabilityHook.
func (s *Service) PluginActivated(ctx context.Context, manifest plugin.PluginManifest, caller plugin.Caller) {
	if !declaresMailCal(manifest.Capabilities) {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider := NewBridgeProvider(caller)
	caps, err := provider.Capabilities(probeCtx, abi.AccountRef{ID: probeAccountID})
	if err != nil {
		slog.Warn("mailcal: provider plugin did not answer the capabilities probe; ignoring it",
			"plugin_id", manifest.ID, "error", err)
		return
	}
	if !hasABI(caps.ABI, abi.CapMail) && !hasABI(caps.ABI, abi.CapCalendar) {
		slog.Warn("mailcal: provider plugin supports no known ABI version; ignoring it",
			"plugin_id", manifest.ID, "abi", caps.ABI)
		return
	}
	s.registerProvider(manifest.ID, manifest.Name, manifest.Version, provider, caps)
	s.mu.Lock()
	if e := s.providers[manifest.ID]; e != nil {
		switch {
		case manifest.OAuth != nil:
			e.info.AuthKind = "oauth"
		case manifest.Permissions.Credentials:
			e.info.AuthKind = "password"
		}
		if s.pluginsDir != "" {
			e.info.Verified = certificate.VerifyDir(filepath.Join(s.pluginsDir, manifest.ID))
		}
	}
	s.mu.Unlock()
	if manifest.OAuth != nil {
		s.mu.Lock()
		if e := s.providers[manifest.ID]; e != nil {
			spec := *manifest.OAuth
			e.oauth = &spec
			e.info.OAuthProvider = spec.Provider
		}
		s.mu.Unlock()
	}
}

// OAuthSpec returns the sign-in description a provider plugin declared, if any.
func (s *Service) OAuthSpec(pluginID string) (plugin.PluginOAuth, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e := s.providers[pluginID]; e != nil && e.oauth != nil {
		return *e.oauth, true
	}
	return plugin.PluginOAuth{}, false
}

// PluginDeactivated implements plugin.CapabilityHook.
func (s *Service) PluginDeactivated(_ context.Context, pluginID string) {
	s.mu.Lock()
	delete(s.providers, pluginID)
	s.mu.Unlock()
}

// RegisterProvider adds an in-process provider (tests, evals). Real providers arrive via PluginActivated.
func (s *Service) RegisterProvider(ctx context.Context, pluginID, name string, p Provider) error {
	caps, err := p.Capabilities(ctx, abi.AccountRef{ID: probeAccountID})
	if err != nil {
		return fmt.Errorf("probing provider %s: %w", pluginID, err)
	}
	s.registerProvider(pluginID, name, "builtin", p, caps)
	return nil
}

func (s *Service) registerProvider(pluginID, name, version string, p Provider, caps abi.Capabilities) {
	s.mu.Lock()
	s.providers[pluginID] = &providerEntry{
		info:     ProviderInfo{PluginID: pluginID, Name: name, Version: version, ABI: caps.ABI, Ops: caps.Ops, Limits: caps.Limits},
		provider: p,
		caps:     caps,
	}
	s.mu.Unlock()
	slog.Info("mailcal: provider ready", "plugin_id", pluginID, "abi", caps.ABI, "ops", len(caps.Ops))
}

func declaresMailCal(caps []string) bool {
	for _, c := range caps {
		if strings.HasPrefix(c, "mail@") || strings.HasPrefix(c, "calendar@") {
			return true
		}
	}
	return false
}

func hasABI(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// Providers lists loaded providers, sorted by plugin id.
func (s *Service) Providers() []ProviderInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ProviderInfo, 0, len(s.providers))
	for _, e := range s.providers {
		out = append(out, e.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PluginID < out[j].PluginID })
	return out
}

// Capabilities returns what the provider behind a plugin id declared.
func (s *Service) Capabilities(pluginID string) (abi.Capabilities, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.providers[pluginID]
	if !ok {
		return abi.Capabilities{}, false
	}
	return e.caps, true
}

// binding is a resolved account with its provider.
type binding struct {
	account  Account
	provider Provider
	caps     abi.Capabilities
}

// resolve finds the account and its provider. need is KindMail or KindCalendar. An empty
// accountID is accepted only when exactly one account can serve the request.
func (s *Service) resolve(ctx context.Context, accountID, need string) (binding, error) {
	var acct Account
	if strings.TrimSpace(accountID) == "" {
		all, err := s.accounts.List(ctx)
		if err != nil {
			return binding{}, err
		}
		var fit []Account
		for _, a := range all {
			if (need == KindMail && a.SupportsMail()) || (need == KindCalendar && a.SupportsCalendar()) {
				fit = append(fit, a)
			}
		}
		switch len(fit) {
		case 0:
			return binding{}, fmt.Errorf("no %s account is configured", need)
		case 1:
			acct = fit[0]
		default:
			ids := make([]string, len(fit))
			for i, a := range fit {
				ids[i] = fmt.Sprintf("%s (%s)", a.ID, a.Label)
			}
			return binding{}, fmt.Errorf("several %s accounts exist; pass 'account' as one of: %s", need, strings.Join(ids, ", "))
		}
	} else {
		var err error
		if acct, err = s.accounts.Get(ctx, accountID); err != nil {
			return binding{}, err
		}
		if (need == KindMail && !acct.SupportsMail()) || (need == KindCalendar && !acct.SupportsCalendar()) {
			return binding{}, fmt.Errorf("account %s (%s) does not serve %s", acct.ID, acct.Label, need)
		}
	}
	if acct.Status != StatusOK {
		detail := ""
		if acct.StatusDetail != "" {
			detail = ": " + acct.StatusDetail
		}
		return binding{}, fmt.Errorf("account %s (%s) is %s%s", acct.ID, acct.Label, acct.Status, detail)
	}
	s.mu.RLock()
	entry, ok := s.providers[acct.PluginID]
	s.mu.RUnlock()
	if !ok {
		return binding{}, fmt.Errorf("provider plugin %q for account %s is not loaded or is disabled", acct.PluginID, acct.ID)
	}
	return binding{account: acct, provider: entry.provider, caps: entry.caps}, nil
}

func (b binding) require(export string) error {
	if !b.caps.Supports(export) {
		return abi.Errorf(abi.ErrUnsupported, "the provider for account %s does not support %s", b.account.ID, export)
	}
	return nil
}

var readBackoff = []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

// readCall runs an idempotent provider read with bounded retries, and records an expired
// login on the account so the UI and later calls know about it. Never use it for writes.
func (s *Service) readCall(ctx context.Context, b binding, fn func() error) error {
	var err error
	for attempt := 0; ; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		e := AsError(err)
		if e == nil {
			return err
		}
		switch e.Code {
		case abi.ErrAuthExpired:
			_ = s.accounts.SetStatus(context.WithoutCancel(ctx), b.account.ID, StatusAuthExpired, e.Message)
			return err
		case abi.ErrTransient, abi.ErrRateLimited:
			if attempt >= len(readBackoff) {
				return err
			}
			wait := readBackoff[attempt]
			if e.Code == abi.ErrRateLimited && e.RetryAfterMS > 0 {
				if e.RetryAfterMS > 5000 {
					return err
				}
				wait = time.Duration(e.RetryAfterMS) * time.Millisecond
			}
			if serr := s.sleep(ctx, wait); serr != nil {
				return serr
			}
		default:
			return err
		}
	}
}

// ---- read API -------------------------------------------------------------------

// MailList lists messages.
func (s *Service) MailList(ctx context.Context, accountID string, p abi.MailListParams) (Account, abi.MailListResult, error) {
	b, err := s.resolve(ctx, accountID, KindMail)
	if err != nil {
		return Account{}, abi.MailListResult{}, err
	}
	if err := b.require(abi.ExportMailList); err != nil {
		return b.account, abi.MailListResult{}, err
	}
	var res abi.MailListResult
	err = s.readCall(ctx, b, func() (e error) { res, e = b.provider.MailList(ctx, b.account.Ref(), p); return })
	if err == nil {
		s.observeInbox(ctx, b.account, res.Messages)
	}
	return b.account, res, err
}

// MailSearch searches messages.
func (s *Service) MailSearch(ctx context.Context, accountID string, p abi.MailSearchParams) (Account, abi.MailListResult, error) {
	b, err := s.resolve(ctx, accountID, KindMail)
	if err != nil {
		return Account{}, abi.MailListResult{}, err
	}
	if err := b.require(abi.ExportMailSearch); err != nil {
		return b.account, abi.MailListResult{}, err
	}
	var res abi.MailListResult
	err = s.readCall(ctx, b, func() (e error) { res, e = b.provider.MailSearch(ctx, b.account.Ref(), p); return })
	return b.account, res, err
}

// MailGet fetches one message with its body.
func (s *Service) MailGet(ctx context.Context, accountID, id string) (Account, abi.Message, error) {
	b, err := s.resolve(ctx, accountID, KindMail)
	if err != nil {
		return Account{}, abi.Message{}, err
	}
	if err := b.require(abi.ExportMailGet); err != nil {
		return b.account, abi.Message{}, err
	}
	var res abi.Message
	err = s.readCall(ctx, b, func() (e error) { res, e = b.provider.MailGet(ctx, b.account.Ref(), id); return })
	return b.account, res, err
}

// MailThread fetches a conversation.
func (s *Service) MailThread(ctx context.Context, accountID, threadID string) (Account, abi.Thread, error) {
	b, err := s.resolve(ctx, accountID, KindMail)
	if err != nil {
		return Account{}, abi.Thread{}, err
	}
	if err := b.require(abi.ExportMailGetThread); err != nil {
		return b.account, abi.Thread{}, err
	}
	var res abi.Thread
	err = s.readCall(ctx, b, func() (e error) { res, e = b.provider.MailGetThread(ctx, b.account.Ref(), threadID); return })
	return b.account, res, err
}

// CalendarList lists events.
func (s *Service) CalendarList(ctx context.Context, accountID string, p abi.CalListParams) (Account, abi.CalListResult, error) {
	b, err := s.resolve(ctx, accountID, KindCalendar)
	if err != nil {
		return Account{}, abi.CalListResult{}, err
	}
	if err := b.require(abi.ExportCalListEvents); err != nil {
		return b.account, abi.CalListResult{}, err
	}
	var res abi.CalListResult
	err = s.readCall(ctx, b, func() (e error) { res, e = b.provider.CalendarList(ctx, b.account.Ref(), p); return })
	return b.account, res, err
}

// CalendarGet fetches one event.
func (s *Service) CalendarGet(ctx context.Context, accountID string, p abi.CalGetParams) (Account, abi.Event, error) {
	b, err := s.resolve(ctx, accountID, KindCalendar)
	if err != nil {
		return Account{}, abi.Event{}, err
	}
	if err := b.require(abi.ExportCalGetEvent); err != nil {
		return b.account, abi.Event{}, err
	}
	var res abi.Event
	err = s.readCall(ctx, b, func() (e error) { res, e = b.provider.CalendarGet(ctx, b.account.Ref(), p); return })
	return b.account, res, err
}

// FreeBusy reports when people are busy.
func (s *Service) FreeBusy(ctx context.Context, accountID string, p abi.FreeBusyParams) (Account, abi.FreeBusyResult, error) {
	b, err := s.resolve(ctx, accountID, KindCalendar)
	if err != nil {
		return Account{}, abi.FreeBusyResult{}, err
	}
	if err := b.require(abi.ExportCalFreeBusy); err != nil {
		return b.account, abi.FreeBusyResult{}, err
	}
	var res abi.FreeBusyResult
	err = s.readCall(ctx, b, func() (e error) { res, e = b.provider.CalendarFreeBusy(ctx, b.account.Ref(), p); return })
	return b.account, res, err
}

// ---- drafts ---------------------------------------------------------------------

// ErrReplyUnverified is returned when a draft claims to answer a message the provider cannot confirm.
var ErrReplyUnverified = errors.New("cannot verify the message being replied to")

// CreateDraft validates and stores an immutable draft. When the draft answers a message, the
// provider is asked once, here, for the thread so the draft carries a verified ReplyContext.
func (s *Service) CreateDraft(ctx context.Context, accountID, agentID, traceID string, payload DraftPayload) (Draft, error) {
	b, err := s.resolve(ctx, accountID, KindMail)
	if err != nil {
		return Draft{}, err
	}
	if err := b.require(abi.ExportMailSend); err != nil {
		return Draft{}, err
	}
	var reply *ReplyContext
	if payload.ThreadID != "" && payload.InReplyTo == "" {
		return Draft{}, fmt.Errorf("%w: thread_id is derived from in_reply_to; do not set it on its own", ErrInvalidDraft)
	}
	payload.ThreadID = ""
	if strings.TrimSpace(payload.InReplyTo) != "" {
		if reply, err = s.replyContext(ctx, b, strings.TrimSpace(payload.InReplyTo)); err != nil {
			return Draft{}, err
		}
		payload.InReplyTo = reply.MessageID
		payload.ThreadID = reply.ThreadID
	}
	return s.drafts.Create(ctx, NewDraft{
		AccountID: b.account.ID, CreatedBy: agentID, TraceID: traceID,
		Payload: payload, Reply: reply, MaxRcpt: b.caps.Limits.MaxRecipients,
	})
}

func (s *Service) replyContext(ctx context.Context, b binding, messageID string) (*ReplyContext, error) {
	if err := b.require(abi.ExportMailGet); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReplyUnverified, err)
	}
	if err := b.require(abi.ExportMailGetThread); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReplyUnverified, err)
	}
	var msg abi.Message
	if err := s.readCall(ctx, b, func() (e error) { msg, e = b.provider.MailGet(ctx, b.account.Ref(), messageID); return }); err != nil {
		return nil, fmt.Errorf("%w: message %q: %v", ErrReplyUnverified, messageID, err)
	}
	var thread abi.Thread
	if err := s.readCall(ctx, b, func() (e error) { thread, e = b.provider.MailGetThread(ctx, b.account.Ref(), msg.ThreadID); return }); err != nil {
		return nil, fmt.Errorf("%w: thread %q: %v", ErrReplyUnverified, msg.ThreadID, err)
	}
	seen := map[string]bool{}
	var participants []string
	add := func(a abi.Address) {
		if e, err := abi.NormalizeEmail(a.Email); err == nil && !seen[e] {
			seen[e] = true
			participants = append(participants, e)
		}
	}
	lastSender := ""
	for _, m := range thread.Messages {
		add(m.From)
		for _, a := range m.To {
			add(a)
		}
		for _, a := range m.Cc {
			add(a)
		}
		if e, err := abi.NormalizeEmail(m.From.Email); err == nil && e != b.account.Email {
			lastSender = e
		}
	}
	sort.Strings(participants)
	return &ReplyContext{MessageID: msg.ID, ThreadID: msg.ThreadID, Verified: true, Participants: participants, SenderOfLast: lastSender}, nil
}

// TestResult is the outcome of probing an account.
type TestResult struct {
	OK     bool     `json:"ok"`
	Detail string   `json:"detail,omitempty"`
	Checks []string `json:"checks,omitempty"`
}

// TestAccount probes an account with harmless reads. Unlike normal calls it runs even when the
// account is marked auth_expired, and a fully successful probe returns it to "ok".
func (s *Service) TestAccount(ctx context.Context, id string) (TestResult, error) {
	acct, err := s.accounts.Get(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	s.mu.RLock()
	entry, ok := s.providers[acct.PluginID]
	s.mu.RUnlock()
	if !ok {
		return TestResult{OK: false, Detail: fmt.Sprintf("provider plugin %q is not loaded or is disabled", acct.PluginID)}, nil
	}
	b := binding{account: acct, provider: entry.provider, caps: entry.caps}
	var res TestResult
	fail := func(what string, err error) (TestResult, error) {
		if e := AsError(err); e != nil && e.Code == abi.ErrAuthExpired {
			_ = s.accounts.SetStatus(ctx, id, StatusAuthExpired, e.Message)
		}
		res.OK, res.Detail = false, fmt.Sprintf("%s: %v", what, err)
		return res, nil
	}
	if acct.SupportsMail() {
		if err := b.require(abi.ExportMailList); err != nil {
			return fail("mail", err)
		}
		if _, err := b.provider.MailList(ctx, acct.Ref(), abi.MailListParams{Limit: 1}); err != nil {
			return fail("mail", err)
		}
		res.Checks = append(res.Checks, "mail: listed the inbox")
	}
	if acct.SupportsCalendar() {
		if err := b.require(abi.ExportCalListEvents); err != nil {
			return fail("calendar", err)
		}
		now := time.Now().UTC()
		if _, err := b.provider.CalendarList(ctx, acct.Ref(), abi.CalListParams{From: now, To: now.Add(time.Hour), Limit: 1}); err != nil {
			return fail("calendar", err)
		}
		res.Checks = append(res.Checks, "calendar: listed upcoming events")
	}
	res.OK = true
	if acct.Status != StatusOK {
		_ = s.accounts.SetStatus(ctx, id, StatusOK, "")
	}
	return res, nil
}

// ValidateAccountAgainstProvider checks that the chosen plugin is loaded and declares the
// capability the account kind needs.
func (s *Service) ValidateAccountAgainstProvider(pluginID, kind string) error {
	caps, ok := s.Capabilities(pluginID)
	if !ok {
		return fmt.Errorf("%w: provider plugin %q is not loaded", ErrInvalidAccount, pluginID)
	}
	if (kind == KindMail || kind == KindBoth) && !hasABI(caps.ABI, abi.CapMail) {
		return fmt.Errorf("%w: plugin %q does not provide mail", ErrInvalidAccount, pluginID)
	}
	if (kind == KindCalendar || kind == KindBoth) && !hasABI(caps.ABI, abi.CapCalendar) {
		return fmt.Errorf("%w: plugin %q does not provide a calendar", ErrInvalidAccount, pluginID)
	}
	return nil
}
