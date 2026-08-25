package workspace

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamedProjectionUsesOriginalFilenames(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	dir, err := store.CreateDirectory(ctx, "", "Reports")
	if err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	content := []byte("%PDF-1.7\nnamed projection\n%%EOF")
	node, err := store.Write(ctx, WriteRequest{
		ParentID: dir.ID, Name: "report.pdf", Content: content, MIMEType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("writing pdf: %v", err)
	}
	if node.ExecPath != "Reports/report.pdf" {
		t.Fatalf("exec_path = %q, want Reports/report.pdf", node.ExecPath)
	}
	namedPath := filepath.Join(store.NamedRoot(), filepath.FromSlash(node.ExecPath))
	got, err := os.ReadFile(namedPath)
	if err != nil {
		t.Fatalf("reading named projection: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("named content mismatch: %q", got)
	}
	resolved, err := store.ResolveRef(ctx, "Reports/report.pdf")
	if err != nil || resolved.ID != node.ID {
		t.Fatalf("resolve exec path: %#v err=%v", resolved, err)
	}
	resolved, err = store.ResolveRef(ctx, VirtualRoot+"/Reports/report.pdf")
	if err != nil || resolved.ID != node.ID {
		t.Fatalf("resolve virtual path: %#v err=%v", resolved, err)
	}
	resolved, err = store.ResolveRef(ctx, AgentViewName+"/Reports/report.pdf")
	if err != nil || resolved.ID != node.ID {
		t.Fatalf("resolve scratchpad path: %#v err=%v", resolved, err)
	}
}

func TestNamedProjectionSanitizesUnsafeNames(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	node, err := store.Write(ctx, WriteRequest{Name: `báo cáo: quý? / bản\\nháp`, Content: []byte("xin chào")})
	if err != nil {
		t.Fatalf("writing unsafe name: %v", err)
	}
	if node.ExecPath == "" || strings.ContainsAny(node.ExecPath, `:*?"<>|`) {
		t.Fatalf("exec_path was not sanitized: %q", node.ExecPath)
	}
	if strings.Contains(node.ExecPath, "/") && !strings.Contains(node.ExecPath, "_") {
		t.Fatalf("slash in name should be flattened in exec_path: %q", node.ExecPath)
	}
	if _, err := os.Stat(filepath.Join(store.NamedRoot(), filepath.FromSlash(node.ExecPath))); err != nil {
		t.Fatalf("sanitized named file missing: %v", err)
	}
	if node.Name != `báo cáo: quý? / bản\\nháp` {
		t.Fatalf("canonical name was rewritten: %q", node.Name)
	}
}

func TestNamedProjectionRenameAndDelete(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	node, err := store.Write(ctx, WriteRequest{Name: "notes.txt", Content: []byte("one")})
	if err != nil {
		t.Fatalf("writing file: %v", err)
	}
	oldPath := filepath.Join(store.NamedRoot(), "notes.txt")
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("named file missing: %v", err)
	}
	dir, err := store.CreateDirectory(ctx, "", "Archive")
	if err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	moved, err := store.Rename(ctx, node.ID, dir.ID, "notes.txt", node.Version)
	if err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if moved.ExecPath != "Archive/notes.txt" {
		t.Fatalf("moved exec_path = %q", moved.ExecPath)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old named path still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.NamedRoot(), "Archive", "notes.txt")); err != nil {
		t.Fatalf("moved named file missing: %v", err)
	}
	if err := store.Delete(ctx, moved.ID, 0, false); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.NamedRoot(), "Archive", "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted named file still exists: %v", err)
	}
}

func TestReconcileNamedIngestsAndUpdates(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	node, err := store.Write(ctx, WriteRequest{Name: "report.pdf", Content: []byte("%PDF-1.7\nold\n%%EOF"), MIMEType: "application/pdf"})
	if err != nil {
		t.Fatalf("writing pdf: %v", err)
	}
	namedPDF := filepath.Join(store.NamedRoot(), "report.pdf")
	updated := []byte("%PDF-1.7\nupdated-by-python\n%%EOF")
	if err := os.WriteFile(namedPDF, updated, 0640); err != nil {
		t.Fatalf("overwriting named pdf: %v", err)
	}
	newCSV := filepath.Join(store.NamedRoot(), "analysis", "out.csv")
	if err := os.MkdirAll(filepath.Dir(newCSV), 0750); err != nil {
		t.Fatalf("creating analysis dir: %v", err)
	}
	if err := os.WriteFile(newCSV, []byte("a,b\n1,2\n"), 0640); err != nil {
		t.Fatalf("writing extra csv: %v", err)
	}

	report, err := store.ReconcileNamed(ctx, "agent_system_core")
	if err != nil {
		t.Fatalf("reconciling: %v", err)
	}
	if len(report.Updated) != 1 || report.Updated[0] != node.ID {
		t.Fatalf("unexpected updated ids: %#v", report.Updated)
	}
	if len(report.Created) != 1 {
		t.Fatalf("unexpected created ids: %#v", report.Created)
	}

	_, got, err := store.Read(ctx, node.ID, 0, 0)
	if err != nil || !bytes.Equal(got, updated) {
		t.Fatalf("updated pdf content = %q err=%v", got, err)
	}
	created, err := store.Get(ctx, report.Created[0])
	if err != nil {
		t.Fatalf("getting ingested file: %v", err)
	}
	if created.Name != "out.csv" || created.ExecPath != "analysis/out.csv" {
		t.Fatalf("ingested node = %#v", created)
	}
}

func TestEnsureAgentView(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.Write(ctx, WriteRequest{Name: "hello.txt", Content: []byte("hi")}); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	agentDir := filepath.Join(t.TempDir(), "workspace")
	if err := store.EnsureAgentView(agentDir); err != nil {
		t.Skipf("agent user-workspace view unavailable on this host: %v", err)
	}
	viewFile := filepath.Join(agentDir, AgentViewName, "hello.txt")
	got, err := os.ReadFile(viewFile)
	if err != nil {
		t.Fatalf("reading agent view: %v", err)
	}
	if string(got) != "hi" {
		t.Fatalf("agent view content = %q", got)
	}
}

func TestSanitizeFSName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, wantPrefix string
	}{
		{"report.pdf", "report.pdf"},
		{"CON.txt", "_CON.txt"},
		{"a/b", "a_b"},
		{"...", "unnamed"},
		{".", "unnamed"},
	}
	for _, test := range tests {
		got := sanitizeFSName(test.in)
		if got != test.wantPrefix && !strings.HasPrefix(got, test.wantPrefix) {
			t.Fatalf("sanitizeFSName(%q) = %q, want %q", test.in, got, test.wantPrefix)
		}
	}
}
