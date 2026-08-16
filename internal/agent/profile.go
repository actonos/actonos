package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/memory"
)

// UserProfile represents extracted user preferences, habits, and persona.
type UserProfile struct {
	UserName           string            `json:"user_name"`
	Language           string            `json:"language"` // "en", "vi"
	CommunicationStyle string            `json:"communication_style"` // "concise", "detailed", "technical"
	NamingConventions  string            `json:"naming_conventions"`
	Preferences        map[string]string `json:"preferences"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

// ProceduralPattern represents a stored best practice or workflow optimization rule.
type ProceduralPattern struct {
	ID          string    `json:"id"`
	Domain      string    `json:"domain"` // "coding", "git", "data", "system"
	PatternName string    `json:"pattern_name"`
	Workflow    string    `json:"workflow"` // Instruction pattern or command sequence
	SuccessRate float64   `json:"success_rate"`
	CreatedAt   time.Time `json:"created_at"`
}

// UserProfileManager manages the persistent user profile memory in SQLite and JSON.
type UserProfileManager struct {
	mu           sync.RWMutex
	db           *sql.DB
	configPath   string
	soulPath     string
	memoryMDPath string
	profile      UserProfile
}

// NewUserProfileManager creates a new UserProfileManager.
func NewUserProfileManager(db *memory.DB, dataDir string) (*UserProfileManager, error) {
	configDir := filepath.Join(dataDir, "config")
	workspaceDir := filepath.Join(dataDir, "workspace")
	_ = os.MkdirAll(configDir, 0755)
	_ = os.MkdirAll(workspaceDir, 0755)

	configPath := filepath.Join(configDir, "profile.json")
	soulPath := filepath.Join(configDir, "SOUL.md")
	memoryMDPath := filepath.Join(workspaceDir, "MEMORY.md")

	var sqlDB *sql.DB
	if db != nil {
		sqlDB = db.SQLDB()
	}

	mgr := &UserProfileManager{
		db:           sqlDB,
		configPath:   configPath,
		soulPath:     soulPath,
		memoryMDPath: memoryMDPath,
		profile: UserProfile{
			Language:           "en",
			CommunicationStyle: "concise",
			Preferences:        make(map[string]string),
			UpdatedAt:          time.Now().UTC(),
		},
	}

	if sqlDB != nil {
		if err := mgr.initSchema(); err != nil {
			return nil, err
		}
	}
	mgr.loadFromDisk()

	return mgr, nil
}

func (m *UserProfileManager) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS user_profile (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS procedural_memory (
		id TEXT PRIMARY KEY,
		domain TEXT NOT NULL,
		pattern_name TEXT NOT NULL,
		workflow TEXT NOT NULL,
		success_rate REAL DEFAULT 1.0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := m.db.Exec(schema)
	return err
}

func (m *UserProfileManager) loadFromDisk() {
	if data, err := os.ReadFile(m.configPath); err == nil {
		var p UserProfile
		if err := json.Unmarshal(data, &p); err == nil {
			m.profile = p
		}
	}
}

// GetProfile returns a copy of the current user profile.
func (m *UserProfileManager) GetProfile() UserProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.profile
}

// UpdateProfile updates user profile in memory, disk, and SQLite.
func (m *UserProfileManager) UpdateProfile(ctx context.Context, p UserProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p.UpdatedAt = time.Now().UTC()
	m.profile = p

	// Save to JSON
	data, _ := json.MarshalIndent(p, "", "  ")
	_ = os.WriteFile(m.configPath, data, 0644)

	// Save to SQLite
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO user_profile (key, value, updated_at)
		VALUES ('main_profile', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, string(data), p.UpdatedAt)

	return err
}

// StoreProceduralPattern saves an optimized workflow pattern.
func (m *UserProfileManager) StoreProceduralPattern(ctx context.Context, pattern ProceduralPattern) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO procedural_memory (id, domain, pattern_name, workflow, success_rate, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET workflow = excluded.workflow, success_rate = excluded.success_rate
	`, pattern.ID, pattern.Domain, pattern.PatternName, pattern.Workflow, pattern.SuccessRate, time.Now().UTC())
	return err
}

// GetRelevantPatterns returns procedural workflow patterns for a specific domain.
func (m *UserProfileManager) GetRelevantPatterns(ctx context.Context, domain string) ([]ProceduralPattern, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, domain, pattern_name, workflow, success_rate, created_at
		FROM procedural_memory
		WHERE domain = ? OR domain = 'general'
		ORDER BY success_rate DESC LIMIT 5
	`, domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []ProceduralPattern
	for rows.Next() {
		var p ProceduralPattern
		if err := rows.Scan(&p.ID, &p.Domain, &p.PatternName, &p.Workflow, &p.SuccessRate, &p.CreatedAt); err == nil {
			patterns = append(patterns, p)
		}
	}
	return patterns, nil
}

// GetSoul retrieves the current Agent Soul personality instructions.
func (m *UserProfileManager) GetSoul() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if data, err := os.ReadFile(m.soulPath); err == nil && len(data) > 0 {
		return string(data)
	}

	return `# ActonOS Agent Soul
You are an autonomous AI Agent running on the ActonOS Appliance Kernel.
Your purpose is to assist the user proactively, execute tasks precisely, and respect privacy and system security constraints.
`
}

// SaveSoul updates the SOUL.md personality markdown file.
func (m *UserProfileManager) SaveSoul(ctx context.Context, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return os.WriteFile(m.soulPath, []byte(content), 0644)
}

// GetMemoryMD retrieves the persistent markdown memory diary.
func (m *UserProfileManager) GetMemoryMD() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if data, err := os.ReadFile(m.memoryMDPath); err == nil {
		return string(data)
	}
	return ""
}

// AppendMemoryMD appends a timestamped reflection entry to MEMORY.md.
func (m *UserProfileManager) AppendMemoryMD(ctx context.Context, entry string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, err := os.OpenFile(m.memoryMDPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05")
	formatted := fmt.Sprintf("\n### [%s]\n%s\n", timestamp, entry)
	_, err = f.WriteString(formatted)
	return err
}

