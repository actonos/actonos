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
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// SkillManifest defines metadata stored in `skill.json` or `SKILL.md` YAML frontmatter.
type SkillManifest struct {
	Name         string         `json:"name" yaml:"name"`
	Description  string         `json:"description" yaml:"description"`
	Category     string         `json:"category" yaml:"category"`
	Schema       json.RawMessage `json:"parameters" yaml:"-"`
	RawParams    map[string]any `json:"-" yaml:"parameters"`
	Entrypoint   string         `json:"entrypoint,omitempty" yaml:"entrypoint"`
	Instructions string         `json:"instructions,omitempty" yaml:"instructions"`
}

// SkillTool wraps a script folder or prompt into a callable Tool.
type SkillTool struct {
	skillName    string
	description  string
	category     string
	schema       json.RawMessage
	folderPath   string
	entrypoint   string
	instructions string
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

	return &SkillTool{
		skillName:    "skill_" + strings.ToLower(strings.ReplaceAll(skillName, " ", "_")),
		description:  manifest.Description,
		category:     category,
		schema:       schema,
		folderPath:   folderPath,
		entrypoint:   entrypoint,
		instructions: instructions,
	}, nil
}

func (s *SkillTool) Name() string                     { return s.skillName }
func (s *SkillTool) Description() string              { return s.description }
func (s *SkillTool) Category() string                 { return s.category }
func (s *SkillTool) ParametersSchema() json.RawMessage { return s.schema }
func (s *SkillTool) Instructions() string             { return s.instructions }

func (s *SkillTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
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
		// If bash or python3 failed, return descriptive error
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
		slog.Info("skill loaded and registered", "name", skillTool.Name(), "path", subPath)
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
