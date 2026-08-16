package server

import (
	"io/fs"
	"os"
	"path/filepath"
)

// LayeredFS implements an io/fs.FS that checks an override directory on disk before falling back to embedded assets.
type LayeredFS struct {
	overrideDir string
	embedded    fs.FS
}

// NewLayeredFS creates a new LayeredFS instance.
func NewLayeredFS(overrideDir string, embedded fs.FS) *LayeredFS {
	return &LayeredFS{
		overrideDir: overrideDir,
		embedded:    embedded,
	}
}

// Open attempts to open a file from overrideDir, falling back to embedded.
func (l *LayeredFS) Open(name string) (fs.File, error) {
	if l.overrideDir != "" {
		diskPath := filepath.Join(l.overrideDir, filepath.FromSlash(name))
		if f, err := os.Open(diskPath); err == nil {
			return f, nil
		}
	}

	if l.embedded != nil {
		return l.embedded.Open(name)
	}

	return nil, os.ErrNotExist
}
