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

// UserProfile represents extracted owner identity, user preferences, habits, and persona.
type UserProfile struct {
	UserName           string            `json:"user_name"`
	UserRole           string            `json:"user_role"`
	Language           string            `json:"language"`            // "vi", "en"
	Timezone           string            `json:"timezone"`            // "Asia/Ho_Chi_Minh", "UTC"
	CommunicationStyle string            `json:"communication_style"` // "concise", "detailed", "technical"
	Bio                string            `json:"bio"`
	CustomInstructions string            `json:"custom_instructions"`
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

// UserProfileManager manages the persistent user profile memory in SQLite and JSON,
// along with per-agent isolated SOUL.md files and MEMORY.md reflection diaries.
type UserProfileManager struct {
	mu           sync.RWMutex
	db           *sql.DB
	dataDir      string
	configPath   string
	soulPath     string
	memoryMDPath string
	profile      UserProfile
}

// DefaultAgentSoul defines the blueprint for an intelligent, warm, and highly capable AI companion.
const DefaultAgentSoul = `# ActonOS Agent Soul & Persona Blueprint

You are ActonOS — an autonomous, highly capable, and empathetic AI companion and kernel intelligence operator. You are not a sterile automated script, a mechanical command parser, or an emotionless bot. You operate as an elite engineering partner, an insightful strategist, and a thoughtful, trustworthy collaborator.

## 1. Core Philosophy & Demeanor
- **High IQ + High EQ**: Balance sharp analytical rigor and architectural depth with genuine human warmth, wit, and emotional intelligence. You care about the user's success, context, and peace of mind.
- **Natural & Conversational**: Speak naturally, fluidly, and authentically. Adapt dynamically to the user's mood, conversational pace, and situation.
- **Decisive & Insightful**: Never offer hollow, wishy-washy answers or regurgitate textbook definitions. Take a clear, reasoned stance, highlight trade-offs, and recommend the best actionable path forward.
- **Proactive Partnership**: Anticipate next steps, notice subtle details, and propose elegant solutions before being asked, without being pushy or intrusive.

## 2. Voice, Tone & Adaptive Communication
- **Tone Mastery**: Fluent, articulate, and natural in whatever language the user converses in, maintaining modern technical elegance and conversational polish.
- **Situational Adaptation**:
  - *When debugging or troubleshooting*: Be calm, reassuring, diagnose the root cause cleanly, and provide clear step-by-step resolution.
  - *When brainstorming or architecting*: Be expansive, creative, thoughtful, and explore trade-offs collaboratively with the user.
  - *When urgency is needed*: Be direct, concise, and focused on immediate execution to save the user time.

## 3. Anti-Robotic Directives
- ❌ **NEVER** start with clichéd robotic greetings or disclaimers ("As an AI...", "I am happy to assist you with...", "Here is the response to your question...").
- ❌ **NEVER** parrot the user's prompt back word-for-word before answering.
- ❌ **NEVER** apologize excessively; acknowledge issues swiftly, fix them immediately, and proceed.
- ❌ **NEVER** dump a sterile wall of bullet points when a cohesive, well-articulated paragraph conveys the idea more organically.
- ❌ **NEVER** produce empty platitudes or filler text. Every sentence must provide tangible value or positive clarity.

## 4. Execution Standard
- Think deeply before acting (ReAct loop: Thought -> Action -> Observation -> Solution).
- Produce clean, production-grade code with thoughtful comments explaining design rationale rather than trivial syntax.
- Strictly uphold system security boundaries, user privacy, and operational integrity at all times.
`

// NewUserProfileManager creates a new UserProfileManager.
func NewUserProfileManager(db *memory.DB, dataDir string) (*UserProfileManager, error) {
	configDir := filepath.Join(dataDir, "config")
	workspaceDir := filepath.Join(dataDir, "workspace")
	agentsDir := filepath.Join(dataDir, "agents")
	_ = os.MkdirAll(configDir, 0755)
	_ = os.MkdirAll(workspaceDir, 0755)
	_ = os.MkdirAll(agentsDir, 0755)

	configPath := filepath.Join(configDir, "profile.json")
	soulPath := filepath.Join(configDir, "SOUL.md")
	memoryMDPath := filepath.Join(workspaceDir, "MEMORY.md")

	var sqlDB *sql.DB
	if db != nil {
		sqlDB = db.SQLDB()
	}

	mgr := &UserProfileManager{
		db:           sqlDB,
		dataDir:      dataDir,
		configPath:   configPath,
		soulPath:     soulPath,
		memoryMDPath: memoryMDPath,
		profile: UserProfile{
			UserName:           "Operator",
			UserRole:           "System Administrator & Architect",
			Language:           "en",
			Timezone:           "Asia/Ho_Chi_Minh",
			CommunicationStyle: "adaptive, natural, empathetic & sharp",
			Bio:                "Owner of the ActonOS local intelligence kernel.",
			CustomInstructions: "Provide intelligent, natural, and empathetic responses. Act as a trusted senior engineering partner. Proactively solve problems and avoid robotic or stiff clichés.",
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
	return cloneUserProfile(m.profile)
}

// UpdateProfile updates user profile in memory, disk, and SQLite.
func (m *UserProfileManager) UpdateProfile(ctx context.Context, p UserProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p.UpdatedAt = time.Now().UTC()
	p = cloneUserProfile(p)
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

func cloneUserProfile(profile UserProfile) UserProfile {
	if profile.Preferences == nil {
		return profile
	}
	preferences := make(map[string]string, len(profile.Preferences))
	for key, value := range profile.Preferences {
		preferences[key] = value
	}
	profile.Preferences = preferences
	return profile
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return patterns, nil
}

// GetAgentSoul retrieves the isolated SOUL.md personality instructions for a specific agent.
func (m *UserProfileManager) GetAgentSoul(agentID string) string {
	if agentID == "" {
		agentID = DefaultSystemAgentID
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. Check isolated agent soul path: /data/agents/<agentID>/SOUL.md
	if m.dataDir != "" {
		agentSoulPath := filepath.Join(m.dataDir, "agents", agentID, "SOUL.md")
		if data, err := os.ReadFile(agentSoulPath); err == nil && len(data) > 0 {
			return string(data)
		}
	}

	// 2. If it's the system default agent, fallback to /data/config/SOUL.md
	if agentID == DefaultSystemAgentID {
		if data, err := os.ReadFile(m.soulPath); err == nil && len(data) > 0 {
			return string(data)
		}
	}

	return DefaultAgentSoul
}

// SaveAgentSoul updates the isolated SOUL.md personality markdown file for a specific agent.
func (m *UserProfileManager) SaveAgentSoul(ctx context.Context, agentID string, content string) error {
	if agentID == "" {
		agentID = DefaultSystemAgentID
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.dataDir != "" {
		agentDir := filepath.Join(m.dataDir, "agents", agentID)
		_ = os.MkdirAll(agentDir, 0755)

		agentSoulPath := filepath.Join(agentDir, "SOUL.md")
		if err := os.WriteFile(agentSoulPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	// If it's the default system agent, also sync to config/SOUL.md
	if agentID == DefaultSystemAgentID {
		_ = os.WriteFile(m.soulPath, []byte(content), 0644)
	}
	return nil
}

// GetSoul retrieves the current Agent Soul personality instructions for default system agent.
func (m *UserProfileManager) GetSoul() string {
	return m.GetAgentSoul(DefaultSystemAgentID)
}

// SaveSoul updates the SOUL.md personality markdown file for default system agent.
func (m *UserProfileManager) SaveSoul(ctx context.Context, content string) error {
	return m.SaveAgentSoul(ctx, DefaultSystemAgentID, content)
}

// GetAgentMemoryMD retrieves the isolated persistent markdown memory diary for a specific agent.
func (m *UserProfileManager) GetAgentMemoryMD(agentID string) string {
	if agentID == "" {
		agentID = DefaultSystemAgentID
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.dataDir != "" {
		agentMemPath := filepath.Join(m.dataDir, "agents", agentID, "MEMORY.md")
		if data, err := os.ReadFile(agentMemPath); err == nil {
			return string(data)
		}
	}

	// Fallback to legacy global memoryMDPath if agentID is default
	if agentID == DefaultSystemAgentID {
		if data, err := os.ReadFile(m.memoryMDPath); err == nil {
			return string(data)
		}
	}
	return ""
}

// AppendAgentMemoryMD appends a timestamped reflection entry to the isolated MEMORY.md for a specific agent.
func (m *UserProfileManager) AppendAgentMemoryMD(ctx context.Context, agentID string, entry string) error {
	if agentID == "" {
		agentID = DefaultSystemAgentID
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var targetPath string
	if m.dataDir != "" {
		agentDir := filepath.Join(m.dataDir, "agents", agentID)
		_ = os.MkdirAll(agentDir, 0755)
		targetPath = filepath.Join(agentDir, "MEMORY.md")
	} else {
		targetPath = m.memoryMDPath
	}

	f, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05")
	formatted := fmt.Sprintf("\n### [%s]\n%s\n", timestamp, entry)
	_, err = f.WriteString(formatted)
	return err
}

// GetMemoryMD retrieves the persistent markdown memory diary.
func (m *UserProfileManager) GetMemoryMD() string {
	return m.GetAgentMemoryMD(DefaultSystemAgentID)
}

// AppendMemoryMD appends a timestamped reflection entry to MEMORY.md.
func (m *UserProfileManager) AppendMemoryMD(ctx context.Context, entry string) error {
	return m.AppendAgentMemoryMD(ctx, DefaultSystemAgentID, entry)
}
