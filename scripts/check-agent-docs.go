//go:build ignore

// check-agent-docs fails when the guidance that AI agents read points at files or make targets that
// no longer exist. It scans AGENTS.md, CLAUDE.md, .agents/** and the per-area AGENTS.md files for
// backticked repository paths and `make <target>` mentions.
//
//	go run ./scripts/check-agent-docs.go
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	backtick = regexp.MustCompile("`([^`\\n]+)`")
	makeCmd  = regexp.MustCompile(`\bmake ([a-z][a-z0-9-]*)`)
	target   = regexp.MustCompile(`(?m)^([a-z][a-z0-9-]*):`)
	roots    = []string{"internal/", "cmd/", "web/", "docs/", "scripts/", "evals/", ".agents/", "deploy/", ".github/"}
	rootFile = map[string]bool{"Makefile": true, "AGENTS.md": true, "CLAUDE.md": true, "CHANGELOG.md": true, "README.md": true, "go.mod": true}
)

func main() {
	var files []string
	add := func(p string) {
		if _, err := os.Stat(p); err == nil {
			files = append(files, p)
		}
	}
	add("AGENTS.md")
	add("CLAUDE.md")
	_ = filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "dist", "build", "superpowers":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "AGENTS.md" && p != "AGENTS.md" || strings.HasPrefix(p, ".agents"+string(filepath.Separator)) && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})

	mk, _ := os.ReadFile("Makefile")
	targets := map[string]bool{}
	for _, m := range target.FindAllStringSubmatch(string(mk), -1) {
		targets[m[1]] = true
	}

	var problems []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text := string(raw)
		for _, m := range backtick.FindAllStringSubmatch(text, -1) {
			tok := strings.TrimRight(m[1], ".,;:)")
			if i := strings.IndexAny(tok, " \t"); i >= 0 || strings.ContainsAny(tok, "<>{}*$|=") || strings.Contains(tok, "...") || strings.Contains(tok, "(") {
				continue
			}
			if c := strings.LastIndex(tok, ":"); c > 0 && isDigits(tok[c+1:]) {
				tok = tok[:c]
			}
			if !looksLikePath(tok) {
				continue
			}
			if _, err := os.Stat(strings.TrimSuffix(tok, "/")); err != nil {
				problems = append(problems, fmt.Sprintf("%s: missing path %s", f, tok))
			}
		}
		for _, m := range makeCmd.FindAllStringSubmatch(text, -1) {
			if !targets[m[1]] && !prose[m[1]] {
				problems = append(problems, fmt.Sprintf("%s: no make target %q", f, m[1]))
			}
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, "agent docs reference things that do not exist:")
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "  "+p)
		}
		os.Exit(1)
	}
	fmt.Printf("check-agent-docs: %d files ok\n", len(files))
}

// prose lists words that follow "make" in ordinary English ("make changes").
var prose = map[string]bool{"changes": true, "sure": true, "it": true, "a": true, "the": true, "sense": true, "use": true, "them": true, "no": true}

func looksLikePath(tok string) bool {
	if rootFile[tok] {
		return true
	}
	for _, r := range roots {
		if strings.HasPrefix(tok, r) {
			return true
		}
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
