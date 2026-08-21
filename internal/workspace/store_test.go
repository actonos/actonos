package workspace

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(context.Background(), db, t.TempDir())
	if err != nil {
		t.Fatalf("creating workspace store: %v", err)
	}
	return store
}

func TestStoreRoundTripArbitraryNamesAndFormats(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		content  []byte
		wantKind string
	}{
		{name: `báo cáo: quý? / bản\\nháp`, content: []byte("xin chào"), wantKind: "text"},
		{name: "không có phần mở rộng", content: []byte{0x00, 0xff, 0x10, 0x80}, wantKind: "binary"},
		{name: "CON.*", content: []byte(`{"valid":true}`), wantKind: "text"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node, err := store.Write(ctx, WriteRequest{Name: test.name, Content: test.content, ActorID: "test"})
			if err != nil {
				t.Fatalf("writing file: %v", err)
			}
			if node.Name != test.name {
				t.Fatalf("name was rewritten: got %q want %q", node.Name, test.name)
			}
			if node.VirtualPath == "" || !strings.HasPrefix(node.VirtualPath, VirtualRoot+"/") {
				t.Fatalf("unexpected virtual path %q", node.VirtualPath)
			}
			if strings.Contains(node.VirtualPath, `\\`) {
				t.Fatalf("virtual path exposed an unescaped separator: %q", node.VirtualPath)
			}
			readNode, got, err := store.Read(ctx, node.ID, 0, 0)
			if err != nil {
				t.Fatalf("reading file: %v", err)
			}
			if !bytes.Equal(got, test.content) {
				t.Fatalf("content mismatch: got %x want %x", got, test.content)
			}
			if kind := MediaKind(readNode.MIMEType); kind != test.wantKind {
				t.Fatalf("kind = %q, want %q (mime %q)", kind, test.wantKind, readNode.MIMEType)
			}
		})
	}
}

func TestStoreOptimisticConcurrencySearchAndDelete(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	dir, err := store.CreateDirectory(ctx, "", "Tài liệu")
	if err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	node, err := store.Write(ctx, WriteRequest{ParentID: dir.ID, Name: "notes", Content: []byte("ActonOS durable workspace")})
	if err != nil {
		t.Fatalf("writing file: %v", err)
	}
	updated, err := store.Write(ctx, WriteRequest{ID: node.ID, Content: []byte("ActonOS SQLite workspace"), ExpectedVersion: node.Version})
	if err != nil {
		t.Fatalf("updating file: %v", err)
	}
	if _, err := store.Write(ctx, WriteRequest{ID: node.ID, Content: []byte("stale"), ExpectedVersion: node.Version}); !errors.Is(err, ErrVersion) {
		t.Fatalf("stale update error = %v, want ErrVersion", err)
	}
	results, err := store.Search(ctx, "notes", "", 20)
	if err != nil {
		t.Fatalf("searching file metadata: %v", err)
	}
	if len(results) != 1 || results[0].ID != node.ID {
		t.Fatalf("unexpected search results: %#v", results)
	}
	if err := store.Delete(ctx, dir.ID, dir.Version, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("non-recursive directory delete error = %v, want ErrConflict", err)
	}
	if err := store.Delete(ctx, dir.ID, 0, true); err != nil {
		t.Fatalf("recursive delete: %v", err)
	}
	if _, err := store.Get(ctx, updated.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted child error = %v, want ErrNotFound", err)
	}
}

func TestStoreReadRange(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	node, err := store.Write(ctx, WriteRequest{Name: "large.pdf", Content: []byte("0123456789"), MIMEType: "application/pdf"})
	if err != nil {
		t.Fatalf("writing file: %v", err)
	}
	_, content, err := store.Read(ctx, node.ID, 3, 4)
	if err != nil {
		t.Fatalf("reading file range: %v", err)
	}
	if got, want := string(content), "3456"; got != want {
		t.Fatalf("range content = %q, want %q", got, want)
	}
	_, content, err = store.Read(ctx, node.ID, 8, 0)
	if err != nil {
		t.Fatalf("reading file tail: %v", err)
	}
	if got, want := string(content), "89"; got != want {
		t.Fatalf("tail content = %q, want %q", got, want)
	}
}

func TestStorePersistsOnlyRelativePathInDatabase(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	directory, err := store.CreateDirectory(ctx, "", "Reports")
	if err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	content := []byte("%PDF-1.7\nfilesystem payload\n%%EOF")
	node, err := store.Write(ctx, WriteRequest{
		ParentID: directory.ID, Name: "report.pdf", Content: content, MIMEType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("writing file: %v", err)
	}

	var relativePath string
	if err := store.db.QueryRowContext(ctx, `SELECT relative_path FROM workspace_nodes WHERE id = ?`, node.ID).Scan(&relativePath); err != nil {
		t.Fatalf("reading stored relative path: %v", err)
	}
	wantRelative := filepath.ToSlash(filepath.Join(directory.ID, node.ID))
	if relativePath != wantRelative {
		t.Fatalf("relative path = %q, want %q", relativePath, wantRelative)
	}
	physicalContent, err := os.ReadFile(filepath.Join(store.root, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("reading physical file: %v", err)
	}
	if !bytes.Equal(physicalContent, content) {
		t.Fatalf("physical content = %q, want %q", physicalContent, content)
	}
	updatedContent := []byte("%PDF-1.7\nupdated filesystem payload\n%%EOF")
	updated, err := store.Write(ctx, WriteRequest{ID: node.ID, Content: updatedContent, ExpectedVersion: node.Version})
	if err != nil {
		t.Fatalf("updating physical file: %v", err)
	}
	physicalContent, err = os.ReadFile(filepath.Join(store.root, filepath.FromSlash(relativePath)))
	if err != nil || !bytes.Equal(physicalContent, updatedContent) {
		t.Fatalf("updated physical content = %q, err=%v", physicalContent, err)
	}

	newDirectory, err := store.CreateDirectory(ctx, "", "Archive")
	if err != nil {
		t.Fatalf("creating move target: %v", err)
	}
	moved, err := store.Rename(ctx, node.ID, newDirectory.ID, node.Name, updated.Version)
	if err != nil {
		t.Fatalf("moving file: %v", err)
	}
	var movedRelativePath string
	if err := store.db.QueryRowContext(ctx, `SELECT relative_path FROM workspace_nodes WHERE id = ?`, moved.ID).Scan(&movedRelativePath); err != nil {
		t.Fatalf("reading moved relative path: %v", err)
	}
	wantMovedRelative := filepath.ToSlash(filepath.Join(newDirectory.ID, node.ID))
	if movedRelativePath != wantMovedRelative {
		t.Fatalf("moved relative path = %q, want %q", movedRelativePath, wantMovedRelative)
	}
	if _, err := os.Stat(filepath.Join(store.root, filepath.FromSlash(relativePath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old physical path still exists: %v", err)
	}
	if movedContent, err := os.ReadFile(filepath.Join(store.root, filepath.FromSlash(movedRelativePath))); err != nil || !bytes.Equal(movedContent, updatedContent) {
		t.Fatalf("moved physical content = %q, err=%v", movedContent, err)
	}

	rows, err := store.db.QueryContext(ctx, `PRAGMA table_info(workspace_nodes)`)
	if err != nil {
		t.Fatalf("inspect revisions schema: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan revisions schema: %v", err)
		}
		if name == "content" {
			t.Fatal("workspace_nodes must not contain a content column")
		}
	}
	if err := store.Delete(ctx, moved.ID, moved.Version, false); err != nil {
		t.Fatalf("deleting physical file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.root, filepath.FromSlash(movedRelativePath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted physical path still exists: %v", err)
	}
}

func TestStoreRejectsLegacyBlobSchema(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening legacy database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE workspace_nodes (
		id TEXT PRIMARY KEY, parent_id TEXT, name TEXT, node_type TEXT, mime_type TEXT,
		size_bytes INTEGER, content_hash TEXT, version INTEGER, created_at TEXT, updated_at TEXT, deleted_at TEXT
	)`); err != nil {
		t.Fatalf("creating legacy schema: %v", err)
	}
	if _, err := NewStore(context.Background(), db, t.TempDir()); err == nil || !strings.Contains(err.Error(), "legacy workspace BLOB schema") {
		t.Fatalf("legacy schema error = %v", err)
	}
}

func TestImportLegacySeparatesAgentWorkspaceAndIsIdempotent(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	root := t.TempDir()
	legacy := filepath.Join(root, "workspace")
	agents := filepath.Join(root, "agents")
	if err := os.MkdirAll(filepath.Join(legacy, "agent_research", "scratch"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(legacy, "user docs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "agent_research", "scratch", "private.txt"), []byte("private"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "user docs", "shared.bin"), []byte{0, 1, 2}, 0644); err != nil {
		t.Fatal(err)
	}

	report, err := store.ImportLegacy(ctx, legacy, agents, []string{"agent_research"})
	if err != nil {
		t.Fatalf("importing legacy workspace: %v", err)
	}
	if report.ImportedFiles != 1 || report.CopiedAgentFiles != 1 {
		t.Fatalf("unexpected migration report: %#v", report)
	}
	privateData, err := os.ReadFile(filepath.Join(agents, "agent_research", "workspace", "scratch", "private.txt"))
	if err != nil || string(privateData) != "private" {
		t.Fatalf("private agent file was not copied: data=%q err=%v", privateData, err)
	}
	rootNodes, err := store.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rootNodes) != 1 || rootNodes[0].Name != "user docs" {
		t.Fatalf("agent directory leaked into user namespace: %#v", rootNodes)
	}
	second, err := store.ImportLegacy(ctx, legacy, agents, []string{"agent_research"})
	if err != nil || !second.AlreadyCompleted {
		t.Fatalf("second migration was not idempotent: report=%#v err=%v", second, err)
	}
}
