package server

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

var registryRow = regexp.MustCompile("^\\|\\s*((?:`[A-Z]+`/?)+)\\s*\\|\\s*`(/api/[^`]*)`")

// registryRoutes reads the "METHOD path" pairs listed in the source registry.
func registryRoutes(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".agents", "rules", "source-registry.md"))
	if err != nil {
		t.Fatalf("reading the source registry: %v", err)
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		m := registryRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		for _, method := range strings.Split(strings.ReplaceAll(m[1], "`", ""), "/") {
			if method != "" {
				out[method+" "+normalizeRoute(m[2])] = true
			}
		}
	}
	return out
}

// normalizeRoute drops trailing slashes so "/api/x/" and "/api/x" compare equal.
func normalizeRoute(p string) string {
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// TestSourceRegistryListsEveryRoute keeps .agents/rules/source-registry.md honest: a route added to the
// router without a registry row (or a row for a route that no longer exists) fails here, so
// agents can trust the registry instead of re-reading router.go.
func TestSourceRegistryListsEveryRoute(t *testing.T) {
	srv := newTestServer(t)
	registry := registryRoutes(t)
	if len(registry) < 50 {
		t.Fatalf("parsed only %d registry rows; the table format changed", len(registry))
	}
	live := map[string]bool{}
	err := chi.Walk(srv.Router(), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/api/") {
			live[method+" "+normalizeRoute(route)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var missing, stale []string
	for r := range live {
		if !registry[r] {
			missing = append(missing, r)
		}
	}
	for r := range registry {
		if !live[r] {
			stale = append(stale, r)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("source registry is out of date.\nrouter has, registry lacks (%d):\n  %s\nregistry has, router lacks (%d):\n  %s",
			len(missing), strings.Join(missing, "\n  "), len(stale), strings.Join(stale, "\n  "))
	}
}

var apiDocHeading = regexp.MustCompile("^#{2,4}\\s+`([A-Z]+)\\s+(/api/[^`\\s?]*)[^`]*`")

// apiDocRoutes reads the "METHOD path" headings of docs/API.md.
func apiDocRoutes(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "API.md"))
	if err != nil {
		t.Fatalf("reading docs/API.md: %v", err)
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if m := apiDocHeading.FindStringSubmatch(line); m != nil {
			out[m[1]+" "+normalizeRoute(m[2])] = true
		}
	}
	return out
}

// TestAPIDocListsEveryRoute keeps docs/API.md complete: every protected or public /api route has a
// "### `METHOD /api/path`" heading, and no heading documents a route that was removed.
func TestAPIDocListsEveryRoute(t *testing.T) {
	srv := newTestServer(t)
	doc := apiDocRoutes(t)
	if len(doc) < 100 {
		t.Fatalf("parsed only %d API.md headings; the heading format changed", len(doc))
	}
	live := map[string]bool{}
	_ = chi.Walk(srv.Router(), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/api/") {
			live[method+" "+normalizeRoute(route)] = true
		}
		return nil
	})
	var missing, stale []string
	for r := range live {
		if !doc[r] {
			missing = append(missing, r)
		}
	}
	for r := range doc {
		if !live[r] {
			stale = append(stale, r)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("docs/API.md is out of date.\nrouter has, docs lack (%d):\n  %s\ndocs have, router lacks (%d):\n  %s",
			len(missing), strings.Join(missing, "\n  "), len(stale), strings.Join(stale, "\n  "))
	}
}
