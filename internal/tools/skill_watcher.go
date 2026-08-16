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
)

// SkillManifest defines metadata stored in `skill.json`.
type SkillManifest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"parameters"`
	Entrypoint  string          `json:"entrypoint,omitempty"` // e.g. "run.sh", "run.py", "main.js"
}

// SkillTool wraps a script folder into a callable Tool.
type SkillTool struct {
	skillName   string
	description string
	schema      json.RawMessage
	folderPath  string
	entrypoint  string
}

// NewSkillTool creates a SkillTool from a folder containing skill.json and an executable script.
func NewSkillTool(folderPath string) (*SkillTool, error) {
	manifestPath := filepath.Join(folderPath, "skill.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading skill.json: %w", err)
	}

	var manifest SkillManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing skill.json: %w", err)
	}

	skillName := manifest.Name
	if skillName == "" {
		skillName = filepath.Base(folderPath)
	}

	entrypoint := manifest.Entrypoint
	if entrypoint == "" {
		// Auto-detect entrypoint
		candidates := []string{"run.sh", "run.py", "main.py", "run.js", "main.sh"}
		for _, c := range candidates {
			if _, err := os.Stat(filepath.Join(folderPath, c)); err == nil {
				entrypoint = c
				break
			}
		}
	}

	if entrypoint == "" {
		return nil, errors.New("no executable entrypoint found in skill folder")
	}

	schema := manifest.Schema
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type": "object", "properties": {}}`)
	}

	return &SkillTool{
		skillName:   "skill_" + skillName,
		description: manifest.Description,
		schema:      schema,
		folderPath:  folderPath,
		entrypoint:  entrypoint,
	}, nil
}

func (s *SkillTool) Name() string                     { return s.skillName }
func (s *SkillTool) Description() string              { return s.description }
func (s *SkillTool) Category() string                 { return "skill" }
func (s *SkillTool) ParametersSchema() json.RawMessage { return s.schema }

func (s *SkillTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	scriptPath := filepath.Join(s.folderPath, s.entrypoint)

	var cmd *exec.Cmd
	if strings.HasSuffix(s.entrypoint, ".py") {
		cmd = exec.CommandContext(ctx, "python3", scriptPath)
	} else if strings.HasSuffix(s.entrypoint, ".js") {
		cmd = exec.CommandContext(ctx, "node", scriptPath)
	} else {
		// Default to bash or direct execution
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

// SkillWatcher watches the `/data/skills/` folder and hot-reloads skills using fsnotify.
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

	if err := os.MkdirAll(w.dir, 0755); err != nil {
		slog.Error("creating skills dir", "error", err)
		return
	}

	entries, err := os.ReadDir(w.dir)
	if err != nil {
		slog.Error("reading skills dir", "error", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			folderPath := filepath.Join(w.dir, entry.Name())
			w.loadSkillFolder(folderPath)
		}
	}
}

func (w *SkillWatcher) loadSkillFolder(folderPath string) {
	tool, err := NewSkillTool(folderPath)
	if err != nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if oldTool, exists := w.skills[tool.Name()]; exists {
		w.registry.Unregister(oldTool.Name())
	}

	w.skills[tool.Name()] = tool
	_ = w.registry.Register(tool)
	slog.Info("registered skill tool", "name", tool.Name(), "path", folderPath)
}

// Start launches the background fsnotify watcher loop.
func (w *SkillWatcher) Start() error {
	w.ScanAll()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating fsnotify watcher: %w", err)
	}
	w.watcher = watcher

	if err := w.watcher.Add(w.dir); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("watching skills dir: %w", err)
	}

	go w.watchLoop()
	return nil
}

func (w *SkillWatcher) watchLoop() {
	debounceTimer := time.NewTimer(time.Hour)
	debounceTimer.Stop()

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
				// Debounce re-scan
				debounceTimer.Reset(200 * time.Millisecond)
			}
		case <-debounceTimer.C:
			w.ScanAll()
		case <-w.stopCh:
			return
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("skills watcher error", "error", err)
		}
	}
}

// Stop terminates the skill watcher.
func (w *SkillWatcher) Stop() {
	close(w.stopCh)
	if w.watcher != nil {
		_ = w.watcher.Close()
	}
}
