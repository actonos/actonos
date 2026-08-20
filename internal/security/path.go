package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrPathEscape indicates that a requested path is outside the configured root.
var ErrPathEscape = errors.New("path escapes authorized root")

// ResolvePathWithBase resolves a relative path starting from baseDir while ensuring
// the resulting canonical path remains strictly contained within allowedRoot.
func ResolvePathWithBase(allowedRoot, baseDir, requested string, allowMissing bool) (string, error) {
	if filepath.IsAbs(requested) {
		return "", ErrPathEscape
	}

	absAllowedRoot, err := filepath.Abs(allowedRoot)
	if err != nil {
		return "", fmt.Errorf("resolving root: %w", err)
	}
	absAllowedRoot = canonicalizePotentialPath(absAllowedRoot)

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolving base dir: %w", err)
	}
	absBaseDir = canonicalizePotentialPath(absBaseDir)

	candidate := filepath.Join(absBaseDir, filepath.Clean(requested))
	checkPath := candidate
	if allowMissing {
		checkPath = nearestExistingParent(candidate)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(checkPath); resolveErr == nil {
		if allowMissing && checkPath != candidate {
			rel, relErr := filepath.Rel(checkPath, candidate)
			if relErr != nil {
				return "", fmt.Errorf("resolving requested path: %w", relErr)
			}
			candidate = filepath.Join(resolved, rel)
		} else {
			candidate = resolved
		}
	} else if !allowMissing {
		return "", fmt.Errorf("resolving requested path: %w", resolveErr)
	}

	rel, err := filepath.Rel(absAllowedRoot, candidate)
	if err != nil {
		return "", fmt.Errorf("checking path containment: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", ErrPathEscape
	}
	return candidate, nil
}

// ResolvePath resolves a relative path beneath root while preventing absolute
// paths, lexical traversal, sibling-prefix confusion, and symlink escapes.
func ResolvePath(root, requested string, allowMissing bool) (string, error) {
	return ResolvePathWithBase(root, root, requested, allowMissing)
}

func canonicalizePotentialPath(path string) string {
	existing := nearestExistingParent(path)
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return path
	}
	if existing == path {
		return resolved
	}
	rel, err := filepath.Rel(existing, path)
	if err != nil {
		return path
	}
	return filepath.Join(resolved, rel)
}

func nearestExistingParent(path string) string {
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		current = parent
	}
}
