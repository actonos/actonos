package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// SkillRequirements defines system prerequisites needed for a skill to run safely.
type SkillRequirements struct {
	Config []string `json:"config,omitempty" yaml:"config,omitempty"`
	Env    []string `json:"env,omitempty" yaml:"env,omitempty"`
	Bins   []string `json:"bins,omitempty" yaml:"bins,omitempty"`
	Tools  []string `json:"tools,omitempty" yaml:"tools,omitempty"`
	OS     []string `json:"os,omitempty" yaml:"os,omitempty"`
}

// SkillMetadata holds arbitrary vendor and tooling metadata from frontmatter.
type SkillMetadata struct {
	OpenClaw struct {
		Emoji    string            `json:"emoji,omitempty" yaml:"emoji,omitempty"`
		Requires SkillRequirements `json:"requires,omitempty" yaml:"requires,omitempty"`
	} `json:"openclaw,omitempty" yaml:"openclaw,omitempty"`
	Requires SkillRequirements `json:"requires,omitempty" yaml:"requires,omitempty"`
}

// SkillManifest defines metadata stored in `skill.json` or `SKILL.md` YAML frontmatter.
type SkillManifest struct {
	Name         string            `json:"name" yaml:"name"`
	Description  string            `json:"description" yaml:"description"`
	Category     string            `json:"category" yaml:"category"`
	Schema       json.RawMessage   `json:"parameters" yaml:"-"`
	RawParams    map[string]any    `json:"-" yaml:"parameters"`
	Entrypoint   string            `json:"entrypoint,omitempty" yaml:"entrypoint"`
	Instructions string            `json:"instructions,omitempty" yaml:"instructions"`
	Metadata     SkillMetadata     `json:"metadata,omitempty" yaml:"metadata"`
	Requires     SkillRequirements `json:"requires,omitempty" yaml:"requires"`
}

// VerifySkillRequirements evaluates system environment against declared requirements.
func VerifySkillRequirements(reqs SkillRequirements) (bool, []string) {
	var missing []string

	// 1. Check OS compatibility
	if len(reqs.OS) > 0 {
		matchedOS := false
		for _, osName := range reqs.OS {
			if strings.EqualFold(osName, runtime.GOOS) {
				matchedOS = true
				break
			}
		}
		if !matchedOS {
			missing = append(missing, fmt.Sprintf("OS '%s' not supported (requires: %s)", runtime.GOOS, strings.Join(reqs.OS, ", ")))
		}
	}

	// 2. Check environment variables
	for _, envVar := range reqs.Env {
		if strings.TrimSpace(os.Getenv(envVar)) == "" {
			missing = append(missing, fmt.Sprintf("Missing environment variable '%s'", envVar))
		}
	}

	// 3. Check CLI executable binaries on PATH
	for _, bin := range reqs.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, fmt.Sprintf("Missing executable binary '%s'", bin))
		}
	}

	return len(missing) == 0, missing
}

func mergeRequirements(reqs ...SkillRequirements) SkillRequirements {
	var merged SkillRequirements
	for _, r := range reqs {
		merged.Config = append(merged.Config, r.Config...)
		merged.Env = append(merged.Env, r.Env...)
		merged.Bins = append(merged.Bins, r.Bins...)
		merged.Tools = append(merged.Tools, r.Tools...)
		merged.OS = append(merged.OS, r.OS...)
	}
	dedup := func(slice []string) []string {
		seen := make(map[string]bool)
		var res []string
		for _, s := range slice {
			s = strings.TrimSpace(s)
			if s != "" && !seen[s] {
				seen[s] = true
				res = append(res, s)
			}
		}
		return res
	}
	merged.Config = dedup(merged.Config)
	merged.Env = dedup(merged.Env)
	merged.Bins = dedup(merged.Bins)
	merged.Tools = dedup(merged.Tools)
	merged.OS = dedup(merged.OS)
	return merged
}

// SkillTool wraps a script folder or prompt into a callable Tool.
type SkillTool struct {
	mu                  sync.RWMutex
	skillName           string
	description         string
	category            string
	schema              json.RawMessage
	folderPath          string
	entrypoint          string
	instructions        string
	requirements        SkillRequirements
	requirementsMet     bool
	missingRequirements []string
	enabled             bool
}

// parseSkillMD parses standard ActonOS Agent SKILL.md markdown with YAML frontmatter.
func parseSkillMD(content []byte) (*SkillManifest, string, error) {
	str := string(content)
	if !strings.HasPrefix(str, "---") {
		return nil, "", errors.New("SKILL.md does not contain leading YAML frontmatter delimiter (---)")
	}

	parts := strings.SplitN(str[3:], "---", 2)
	if len(parts) < 2 {
		return nil, "", errors.New("SKILL.md does not contain closing YAML frontmatter delimiter (---)")
	}

	frontmatter := parts[0]
	markdownBody := strings.TrimSpace(parts[1])

	var manifest SkillManifest
	if err := yaml.Unmarshal([]byte(frontmatter), &manifest); err != nil {
		return nil, "", fmt.Errorf("parsing YAML frontmatter: %w", err)
	}

	if len(manifest.RawParams) > 0 {
		schemaBytes, _ := json.Marshal(manifest.RawParams)
		manifest.Schema = json.RawMessage(schemaBytes)
	}

	manifest.Instructions = markdownBody
	return &manifest, markdownBody, nil
}

// NewSkillTool creates a SkillTool from a folder containing SKILL.md or skill.json.
func NewSkillTool(folderPath string) (*SkillTool, error) {
	var manifest *SkillManifest
	var instructions string

	// 1. Try SKILL.md or skill.md first (ActonOS standard)
	skillMDPath := filepath.Join(folderPath, "SKILL.md")
	if _, err := os.Stat(skillMDPath); errors.Is(err, os.ErrNotExist) {
		skillMDPath = filepath.Join(folderPath, "skill.md")
	}

	if mdData, err := os.ReadFile(skillMDPath); err == nil {
		m, body, parseErr := parseSkillMD(mdData)
		if parseErr == nil {
			manifest = m
			instructions = body
		}
	}

	// 2. Fallback to skill.json
	if manifest == nil {
		jsonPath := filepath.Join(folderPath, "skill.json")
		jsonData, err := os.ReadFile(jsonPath)
		if err != nil {
			return nil, fmt.Errorf("neither SKILL.md nor skill.json found in %s", folderPath)
		}
		var m SkillManifest
		if err := json.Unmarshal(jsonData, &m); err != nil {
			return nil, fmt.Errorf("parsing skill.json: %w", err)
		}
		manifest = &m
	}

	skillName := manifest.Name
	if skillName == "" {
		skillName = filepath.Base(folderPath)
	}

	category := manifest.Category
	if category == "" {
		category = "skill"
	}

	entrypoint := manifest.Entrypoint
	if entrypoint == "" {
		candidates := []string{"run.sh", "run.py", "main.py", "run.js", "main.js", "run.bat"}
		for _, c := range candidates {
			if _, err := os.Stat(filepath.Join(folderPath, c)); err == nil {
				entrypoint = c
				break
			}
		}
	}

	schema := manifest.Schema
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type": "object", "properties": {}}`)
	}

	// Calculate requirements and initial validation
	reqs := mergeRequirements(manifest.Requires, manifest.Metadata.Requires, manifest.Metadata.OpenClaw.Requires)
	reqsMet, missing := VerifySkillRequirements(reqs)

	// Check if skill is disabled by .disabled marker file
	disabledMarker := filepath.Join(folderPath, ".disabled")
	enabled := true
	if _, err := os.Stat(disabledMarker); err == nil {
		enabled = false
	}

	return &SkillTool{
		skillName:           "skill_" + strings.ToLower(strings.ReplaceAll(skillName, " ", "_")),
		description:         manifest.Description,
		category:            category,
		schema:              schema,
		folderPath:          folderPath,
		entrypoint:          entrypoint,
		instructions:        instructions,
		requirements:        reqs,
		requirementsMet:     reqsMet,
		missingRequirements: missing,
		enabled:             enabled,
	}, nil
}

func (s *SkillTool) Name() string                     { return s.skillName }
func (s *SkillTool) Description() string              { return s.description }
func (s *SkillTool) Category() string                 { return s.category }
func (s *SkillTool) ParametersSchema() json.RawMessage { return s.schema }
func (s *SkillTool) Instructions() string             { return s.instructions }
func (s *SkillTool) FolderPath() string               { return s.folderPath }

func (s *SkillTool) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

func (s *SkillTool) SetEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	markerPath := filepath.Join(s.folderPath, ".disabled")
	if enabled {
		_ = os.Remove(markerPath)
	} else {
		if f, err := os.Create(markerPath); err == nil {
			_ = f.Close()
		} else {
			return fmt.Errorf("creating disabled marker: %w", err)
		}
	}

	s.enabled = enabled
	return nil
}

func (s *SkillTool) Requirements() SkillRequirements {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.requirements
}

func (s *SkillTool) RequirementsMet() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.requirementsMet
}

func (s *SkillTool) MissingRequirements() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]string, len(s.missingRequirements))
	copy(res, s.missingRequirements)
	return res
}

func (s *SkillTool) CheckRequirements() (bool, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	met, missing := VerifySkillRequirements(s.requirements)
	s.requirementsMet = met
	s.missingRequirements = missing
	return met, missing
}

func (s *SkillTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("skill '%s' is currently disabled", s.skillName)
	}

	if met, missing := s.CheckRequirements(); !met {
		return nil, fmt.Errorf("skill '%s' requirements not satisfied: %s", s.skillName, strings.Join(missing, "; "))
	}

	if s.entrypoint == "" {
		// Pure prompt / instruction skill
		return &ToolResult{
			Content: fmt.Sprintf("[Skill %s Instructions]\n%s\n\nInput Context: %s", s.skillName, s.instructions, string(inputJSON)),
		}, nil
	}

	scriptPath := filepath.Join(s.folderPath, s.entrypoint)
	var cmd *exec.Cmd

	if strings.HasSuffix(s.entrypoint, ".py") {
		cmd = exec.CommandContext(ctx, "python3", scriptPath)
	} else if strings.HasSuffix(s.entrypoint, ".js") {
		cmd = exec.CommandContext(ctx, "node", scriptPath)
	} else if strings.HasSuffix(s.entrypoint, ".bat") {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/c", scriptPath)
	} else {
		cmd = exec.CommandContext(ctx, "bash", scriptPath)
	}

	cmd.Dir = s.folderPath
	cmd.Stdin = bytes.NewReader(inputJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("skill execution failed: %w (stderr: %s)", err, stderr.String())
	}

	return &ToolResult{
		Content: stdout.String(),
	}, nil
}

// SkillWatcher watches `/data/skills/` and hot-reloads skills via fsnotify.
type SkillWatcher struct {
	mu       sync.RWMutex
	registry *ToolRegistry
	skills   map[string]*SkillTool
	dir      string
	watcher  *fsnotify.Watcher
	stopCh   chan struct{}
}

// NewSkillWatcher creates a SkillWatcher instance.
func NewSkillWatcher(registry *ToolRegistry, skillsDir string) *SkillWatcher {
	return &SkillWatcher{
		registry: registry,
		skills:   make(map[string]*SkillTool),
		dir:      skillsDir,
		stopCh:   make(chan struct{}),
	}
}

// ScanAll scans all subdirectories in skillsDir and registers valid skills.
func (w *SkillWatcher) ScanAll() {
	if w.dir == "" {
		return
	}

	_ = os.MkdirAll(w.dir, 0755)
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		slog.Error("reading skills directory", "path", w.dir, "error", err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subPath := filepath.Join(w.dir, entry.Name())
		skillTool, err := NewSkillTool(subPath)
		if err != nil {
			continue
		}

		w.mu.Lock()
		w.skills[skillTool.Name()] = skillTool
		w.mu.Unlock()

		_ = w.registry.Register(skillTool)
		slog.Info("skill loaded and registered", "name", skillTool.Name(), "path", subPath, "enabled", skillTool.IsEnabled(), "requirementsMet", skillTool.RequirementsMet())
	}
}

// Start begins fsnotify folder monitoring.
func (w *SkillWatcher) Start() error {
	w.ScanAll()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating fsnotify watcher: %w", err)
	}
	w.watcher = watcher

	if err := watcher.Add(w.dir); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("watching %s: %w", w.dir, err)
	}

	go w.watchLoop()
	return nil
}

func (w *SkillWatcher) watchLoop() {
	for {
		select {
		case <-w.stopCh:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) {
				time.Sleep(100 * time.Millisecond) // Debounce
				w.ScanAll()
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("fsnotify error in skills watcher", "error", err)
		}
	}
}

// Stop cleanly terminates the watcher.
func (w *SkillWatcher) Stop() {
	close(w.stopCh)
	if w.watcher != nil {
		_ = w.watcher.Close()
	}
}

