package memory

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// WorkspaceWatcher catches file mutations that bypass the HTTP API and native tools.
type WorkspaceWatcher struct {
	root    string
	service *EmbeddingService
	watcher *fsnotify.Watcher
}

func NewWorkspaceWatcher(root string, service *EmbeddingService) (*WorkspaceWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		watcher.Close()
		return nil, err
	}
	return &WorkspaceWatcher{root: filepath.Clean(absRoot), service: service, watcher: watcher}, nil
}

func (w *WorkspaceWatcher) Start(ctx context.Context) error {
	if err := os.MkdirAll(w.root, 0755); err != nil {
		return err
	}
	if err := w.addRecursive(w.root); err != nil {
		return err
	}
	go w.loop(ctx)
	go w.backfill(ctx, w.root)
	return nil
}

func (w *WorkspaceWatcher) Close() error { return w.watcher.Close() }

func (w *WorkspaceWatcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && ignoredEmbeddingPath(path) {
				return filepath.SkipDir
			}
			return w.watcher.Add(path)
		}
		return nil
	})
}

func (w *WorkspaceWatcher) backfill(ctx context.Context, root string) {
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != w.root && ignoredEmbeddingPath(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !ignoredEmbeddingPath(path) {
			_ = w.service.EnqueueFile(ctx, path, "", "shared", EmbeddingUpsert)
		}
		return nil
	})
}

func (w *WorkspaceWatcher) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if ignoredEmbeddingPath(event.Name) {
				continue
			}
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = w.addRecursive(event.Name)
					go w.backfill(ctx, event.Name)
					continue
				}
			}
			operation := EmbeddingUpsert
			if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				operation = EmbeddingDelete
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || operation == EmbeddingDelete {
				_ = w.service.EnqueueFile(context.Background(), event.Name, "", "shared", operation)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("workspace embedding watcher error", "error", err)
		}
	}
}

func ignoredEmbeddingPath(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	if name == "" || name == "." || strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") {
		return true
	}
	switch name {
	case "node_modules", ".git", "vectors", "storage", "models":
		return true
	}
	return strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".swp") || strings.HasSuffix(name, ".part")
}
