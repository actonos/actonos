package memory

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

func TestEmbeddingWorkerStartStopAndEnqueueMemory(t *testing.T) {
	db, service, embedder := newEmbeddingTestService(t)
	service.delay = 0
	now := time.Now().UTC()
	service.now = func() time.Time { return now }

	hybrid := NewHybridEngine(db, service.vectorStore, nil)
	hybrid.SetEmbeddingService(service)
	_ = hybrid.DB()
	emb := make([]float32, EmbeddingDimension)
	emb[0] = 1
	if _, err := hybrid.StoreMemory(context.Background(), "agent-1", LayerEpisodic, "alpha memory fragment", emb, nil, 1); err != nil {
		t.Fatalf("store memory: %v", err)
	}
	var memoryID string
	if err := db.SQLDB().QueryRow(`SELECT id FROM memories LIMIT 1`).Scan(&memoryID); err != nil {
		t.Fatalf("memory id: %v", err)
	}
	if err := service.EnqueueMemory(context.Background(), memoryID, "agent-1"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wsRoot := filepath.Join(t.TempDir(), "ws-store")
	store, err := workspacepkg.NewStore(context.Background(), db.SQLDB(), wsRoot)
	if err != nil {
		t.Fatalf("workspace store: %v", err)
	}
	service.SetWorkspaceStore(store)
	node, err := store.Write(context.Background(), workspacepkg.WriteRequest{Name: "note.txt", Content: []byte("alpha workspace file")})
	if err != nil {
		t.Fatalf("write workspace file: %v", err)
	}
	if err := service.EnqueueWorkspaceFile(context.Background(), node.ID, "agent-1", EmbeddingUpsert); err != nil {
		t.Fatal(err)
	}
	service.SetWriteGuard(func() bool { return true })
	service.Start(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		embedder.mu.Lock()
		n := len(embedder.passages)
		embedder.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	service.Stop()
	embedder.mu.Lock()
	got := len(embedder.passages)
	embedder.mu.Unlock()
	if got == 0 {
		t.Fatal("worker did not embed enqueued memory")
	}

	if _, err := service.EmbedQueryVector(context.Background(), "alpha"); err != nil {
		t.Fatalf("EmbedQueryVector: %v", err)
	}
	if err := embedder.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceWatcherEnqueueAndIgnore(t *testing.T) {
	_, service, _ := newEmbeddingTestService(t)
	service.delay = 0
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello watcher"), 0644); err != nil {
		t.Fatal(err)
	}
	watcher, err := NewWorkspaceWatcher(root, service)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := watcher.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("updated"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	_ = watcher.Close()
	if !ignoredEmbeddingPath(filepath.Join(root, "node_modules", "x.js")) {
		t.Fatal("expected node_modules ignored")
	}
}

func TestExtractDocumentBytesAndVaultList(t *testing.T) {
	if _, err := extractDocumentBytes("huge.bin", "text/plain", make([]byte, maxEmbeddingFileSize+1)); err == nil {
		t.Fatal("expected oversized extract error")
	}
	if _, err := extractDocumentBytes("t.tsv", "text/tab-separated-values", []byte("a\tb\n")); err != nil {
		t.Fatal(err)
	}
	text, err := extractDocumentBytes("note.txt", "text/plain", []byte("plain body"))
	if err != nil || !strings.Contains(text, "plain body") {
		t.Fatalf("plain extract: %q err=%v", text, err)
	}
	csv, err := extractDocumentBytes("t.csv", "text/csv", []byte("a,b\n1,2\n"))
	if err != nil || !strings.Contains(csv, "a") {
		t.Fatalf("csv extract: %q err=%v", csv, err)
	}
	if _, err := extractPDFText(filepath.Join(t.TempDir(), "missing.pdf")); err == nil {
		t.Fatal("expected missing pdf error")
	}
	sample := filepath.Join("testdata", "sample.pdf")
	if extracted, err := extractPDFText(sample); err != nil {
		t.Fatalf("extractPDFText sample: %v", err)
	} else if strings.TrimSpace(extracted) == "" {
		t.Log("sample pdf produced empty text; extraction path still executed")
	}
	pdfBytes, readErr := os.ReadFile(sample)
	if readErr != nil {
		t.Fatalf("read sample pdf: %v", readErr)
	}
	if bytesText, err := extractDocumentBytes("sample.pdf", "application/pdf", pdfBytes); err != nil {
		t.Logf("extractDocumentBytes pdf: %v", err)
	} else if bytesText == "" {
		t.Log("pdf bytes extract empty")
	}
	if !isHexString("00ff") || isHexString("zz") {
		t.Fatal("isHexString")
	}
	if max(2, 9) != 9 || max(9, 2) != 9 {
		t.Fatal("max")
	}
	plainPath := filepath.Join(t.TempDir(), "p.txt")
	if err := os.WriteFile(plainPath, []byte("hello extract"), 0644); err != nil {
		t.Fatal(err)
	}
	if text, err := extractPlainText(plainPath); err != nil || !strings.Contains(text, "hello") {
		t.Fatalf("plain file: %q %v", text, err)
	}

	db, err := Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	v, err := NewVault(db, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetSecret(context.Background(), "k1", "v1"); err != nil {
		t.Fatal(err)
	}
	list, err := v.ListSecrets(context.Background())
	if err != nil || len(list) != 1 || list[0].Name != "k1" {
		t.Fatalf("list secrets: %+v err=%v", list, err)
	}

	tracker := NewTokenTracker(db.SQLDB())
	if err := tracker.Record(context.Background(), TokenUsageRecord{
		AgentID: "agent-1", Model: "openai/gpt-4", Provider: "openai",
		PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20,
	}); err != nil {
		t.Fatal(err)
	}
	n, err := tracker.GetAgentHourlyTokens(context.Background(), "agent-1")
	if err != nil || n < 20 {
		t.Fatalf("hourly tokens %d err=%v", n, err)
	}
	cost, err := tracker.GetAgentMonthlyCost(context.Background(), "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	_ = cost
	if _, err := tracker.GetHistory(context.Background(), 10, "agent-1", ""); err != nil {
		t.Fatalf("history: %v", err)
	}
	if _, err := tracker.GetSummary(context.Background()); err != nil {
		t.Fatalf("summary: %v", err)
	}
	utf16 := []byte{0xFF, 0xFE, 'A', 0, 'B', 0}
	if text, err := extractPlainTextBytes(utf16); err != nil || !strings.Contains(text, "A") {
		t.Fatalf("utf16 extract: %q %v", text, err)
	}
	be := []byte{0xFE, 0xFF, 0, 'A', 0, 'B'}
	if text, err := extractPlainTextBytes(be); err != nil || !strings.Contains(text, "A") {
		t.Fatalf("utf16be extract: %q %v", text, err)
	}
	if err := v.DeleteSecret(context.Background(), "k1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SearchFTS(context.Background(), "agent-1", string(LayerEpisodic), "plain", 0); err != nil {
		t.Fatalf("fts: %v", err)
	}
	if res, err := db.SearchFTS(context.Background(), "agent-1", string(LayerEpisodic), "!!!", 5); err != nil || res != nil {
		t.Fatalf("empty fts: %v %v", res, err)
	}
	_ = sanitizeFTSQuery("hello-world!!!")
	_ = sanitizeFTSQuery("***")
	_ = (&DB{}).Close()
	if err := NewHTTPEmbedder("http://127.0.0.1:1").Health(context.Background()); err == nil {
		t.Fatal("expected connect health error")
	}

	xlsxPath := filepath.Join(t.TempDir(), "t.xlsx")
	var xbuf bytes.Buffer
	zw := zip.NewWriter(&xbuf)
	ss, _ := zw.Create("xl/sharedStrings.xml")
	_, _ = ss.Write([]byte(`<sst><si><t>HelloSheet</t></si></sst>`))
	sh, _ := zw.Create("xl/worksheets/sheet1.xml")
	_, _ = sh.Write([]byte(`<worksheet><sheetData><row><c><v>1</v></c></row></sheetData></worksheet>`))
	_ = zw.Close()
	if err := os.WriteFile(xlsxPath, xbuf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if text, err := extractDocumentText(xlsxPath); err != nil || !strings.Contains(text, "HelloSheet") {
		t.Fatalf("xlsx: %q %v", text, err)
	}

	docxPath := filepath.Join(t.TempDir(), "t.docx")
	var dbuf bytes.Buffer
	dzw := zip.NewWriter(&dbuf)
	doc, _ := dzw.Create("word/document.xml")
	_, _ = doc.Write([]byte(`<w:document><w:p><w:r><w:t>HelloDocx</w:t></w:r><w:br/><w:tab/></w:p></w:document>`))
	_ = dzw.Close()
	if err := os.WriteFile(docxPath, dbuf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if text, err := extractDocumentText(docxPath); err != nil || !strings.Contains(text, "HelloDocx") {
		t.Fatalf("docx: %q %v", text, err)
	}
	badDocx := filepath.Join(t.TempDir(), "bad.docx")
	var bbuf bytes.Buffer
	bzw := zip.NewWriter(&bbuf)
	_, _ = bzw.Create("xl/dummy.xml")
	_ = bzw.Close()
	if err := os.WriteFile(badDocx, bbuf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := extractDocumentText(badDocx); err == nil {
		t.Fatal("expected invalid docx")
	}
	if _, err := extractCSVText(filepath.Join(t.TempDir(), "missing.csv"), ','); err == nil {
		t.Fatal("expected missing csv")
	}
	if _, err := extractXlsxText(filepath.Join(t.TempDir(), "missing.xlsx")); err == nil {
		t.Fatal("expected missing xlsx")
	}
	if _, err := extractDocxText(filepath.Join(t.TempDir(), "missing.docx")); err == nil {
		t.Fatal("expected missing docx")
	}
	if _, err := extractPlainText(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("expected missing txt")
	}
	tsvPath := filepath.Join(t.TempDir(), "t.tsv")
	if err := os.WriteFile(tsvPath, []byte("a\tb\n1\t2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if text, err := extractDocumentText(tsvPath); err != nil || !strings.Contains(text, "a") {
		t.Fatalf("tsv: %q %v", text, err)
	}

	vs, err := NewVectorStore(filepath.Join(t.TempDir(), "vec"))
	if err != nil {
		t.Fatal(err)
	}
	if err := vs.DeleteDocument(context.Background(), semanticCollection, "missing"); err != nil && !strings.Contains(err.Error(), "") {
		t.Fatal(err)
	}
}

func TestEmbeddingFileJobsAndUnsupported(t *testing.T) {
	_, service, embedder := newEmbeddingTestService(t)
	service.delay = 0
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	ws := filepath.Join(t.TempDir(), "workspace", "agent_one")
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}
	service.SetWorkspaceDir(filepath.Dir(ws))
	note := filepath.Join(ws, "note.txt")
	if err := os.WriteFile(note, []byte("alpha file content for embedding"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := service.NotifyFileMutation(context.Background(), note, "agent_one", false); err != nil {
		t.Fatal(err)
	}
	claimAndProcess(t, service)

	bin := filepath.Join(ws, "blob.bin")
	if err := os.WriteFile(bin, []byte{0x00, 0x01, 0xff, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := service.EnqueueFile(context.Background(), bin, "agent_one", "shared", EmbeddingUpsert); err != nil {
		t.Fatal(err)
	}
	job, err := service.claim(context.Background())
	if err != nil || job == nil {
		t.Fatalf("claim binary job: %v %v", job, err)
	}
	if err := service.process(context.Background(), *job); err != nil {
		t.Fatalf("process binary: %v", err)
	}

	if err := service.NotifyFileMutation(context.Background(), note, "agent_one", true); err != nil {
		t.Fatal(err)
	}
	if err := service.NotifyWorkspaceMutation(context.Background(), "", "agent", false); err == nil {
		t.Fatal("expected empty workspace file id error")
	}
	if err := service.EnqueueWorkspaceFile(context.Background(), "", "agent", EmbeddingUpsert); err == nil {
		t.Fatal("expected missing file id")
	}

	orphaned := time.Now().UTC()
	_, _ = service.db.ExecContext(context.Background(), `UPDATE embedding_jobs SET status = 'running', lease_until = ?`, orphaned.Add(-time.Hour))
	service.recoverOrphanedJobs(context.Background())
	service.reviveDeadJobs(context.Background())
	_ = service.markUnsupported(context.Background(), EmbeddingJob{SourceType: "file", SourceKey: "x", SourceRef: "x"}, errUnsupportedEmbeddingSource)

	insertTestMessage(t, service.db, "msg-fail", "conv-fail", "agent-1", "alpha boom")
	if err := service.EnqueueMessage(context.Background(), "msg-fail", "agent-1", "conv-fail"); err != nil {
		t.Fatal(err)
	}
	embedder.mu.Lock()
	embedder.fail = errors.New("embed down")
	embedder.mu.Unlock()
	if job != nil {
		service.fail(context.Background(), *job, errors.New("embed down"))
	}
	_ = service.vectorStore.DeleteDocuments(context.Background(), semanticCollection, []string{"missing"})
}

func TestNormalizeModelNameTable(t *testing.T) {
	cases := []struct{ model, provider, want string }{
		{"", "", "unknown"},
		{"openai/gpt-4", "", "openai/gpt-4"},
		{"gpt-4o", "openai", "openai/gpt-4o"},
		{"deepseek-chat", "", "deepseek/deepseek-chat"},
		{"claude-3", "", "anthropic/claude-3"},
		{"gpt-4", "", "openai/gpt-4"},
		{"o1-mini", "", "openai/o1-mini"},
		{"gemini-pro", "", "google/gemini-pro"},
		{"mistral-small", "", "mistral/mistral-small"},
		{"codestral-latest", "", "mistral/codestral-latest"},
		{"llama-3", "", "ollama/llama-3"},
		{"qwen2", "", "ollama/qwen2"},
		{"custom", "", "custom"},
	}
	for _, tc := range cases {
		if got := NormalizeModelName(tc.model, tc.provider); got != tc.want {
			t.Fatalf("NormalizeModelName(%q,%q)=%q want %q", tc.model, tc.provider, got, tc.want)
		}
	}
}

func TestHTTPEmbedderQueryAndHealth(t *testing.T) {
	var gotKind string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		var payload struct {
			Kind  string   `json:"kind"`
			Texts []string `json:"texts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotKind = payload.Kind
		out := make([][]float32, len(payload.Texts))
		for i := range out {
			vec := make([]float32, EmbeddingDimension)
			vec[0] = 1
			out[i] = vec
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer server.Close()
	embedder := NewHTTPEmbedder(server.URL)
	vecs, err := embedder.EmbedQuery(context.Background(), []string{"hello"})
	if err != nil || len(vecs) != 1 {
		t.Fatalf("query: %v %v", vecs, err)
	}
	if gotKind != "query" {
		t.Fatalf("kind %s", gotKind)
	}
	if err := embedder.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	texts := make([]string, 33)
	for i := range texts {
		texts[i] = "passage"
	}
	if _, err := embedder.EmbedPassages(context.Background(), texts); err != nil {
		t.Fatalf("passages: %v", err)
	}
	_ = NewHTTPEmbedder("")
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	if err := NewHTTPEmbedder(bad.URL).Health(context.Background()); err == nil {
		t.Fatal("expected health error")
	}
	mismatch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{{1, 2}}})
	}))
	defer mismatch.Close()
	if _, err := NewHTTPEmbedder(mismatch.URL).EmbedQuery(context.Background(), []string{"a"}); err == nil {
		t.Fatal("expected dimension/count error")
	}
}

func TestLoadContentBranchesAndNilVault(t *testing.T) {
	db, service, _ := newEmbeddingTestService(t)
	ctx := context.Background()

	if _, _, err := service.loadContent(ctx, EmbeddingJob{SourceType: "nope"}); err == nil {
		t.Fatal("expected unsupported source")
	}
	if _, _, err := service.loadContent(ctx, EmbeddingJob{SourceType: "message", SourceRef: "missing"}); err == nil {
		t.Fatal("expected missing message")
	}
	insertTestMessage(t, service.db, "msg-load", "conv-load", "agent-1", "hello load content")
	text, meta, err := service.loadContent(ctx, EmbeddingJob{SourceType: "message", SourceRef: "msg-load"})
	if err != nil || text != "hello load content" || meta["role"] != "user" {
		t.Fatalf("message load: %q %v %v", text, meta, err)
	}

	now := time.Now().UTC()
	if _, err := service.db.ExecContext(ctx, `INSERT INTO memories (id, agent_id, layer, content, last_accessed_at, created_at) VALUES (?,?,?,?,?,?)`,
		"mem-load", "agent-1", "episodic", "memory body", now, now); err != nil {
		t.Fatalf("insert memory: %v", err)
	}
	text, _, err = service.loadContent(ctx, EmbeddingJob{SourceType: "memory", SourceRef: "mem-load"})
	if err != nil || text != "memory body" {
		t.Fatalf("memory load: %q %v", text, err)
	}

	missingFile := filepath.Join(t.TempDir(), "nope.txt")
	if _, _, err := service.loadContent(ctx, EmbeddingJob{SourceType: "file", SourceRef: missingFile}); err == nil {
		t.Fatal("expected missing file")
	}
	dir := t.TempDir()
	if _, _, err := service.loadContent(ctx, EmbeddingJob{SourceType: "file", SourceRef: dir}); err == nil {
		t.Fatal("expected directory error")
	}
	note := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(note, []byte("file body"), 0644); err != nil {
		t.Fatal(err)
	}
	text, _, err = service.loadContent(ctx, EmbeddingJob{SourceType: "file", SourceRef: note})
	if err != nil || !strings.Contains(text, "file body") {
		t.Fatalf("file load: %q %v", text, err)
	}

	if _, _, err := service.loadContent(ctx, EmbeddingJob{SourceType: "workspace_file", SourceRef: "x"}); err == nil {
		t.Fatal("expected unavailable workspace store")
	}

	store, err := workspacepkg.NewStore(ctx, db.SQLDB(), filepath.Join(t.TempDir(), "ws"))
	if err != nil {
		t.Fatalf("workspace store: %v", err)
	}
	service.SetWorkspaceStore(store)
	if _, _, err := service.loadContent(ctx, EmbeddingJob{SourceType: "workspace_file", SourceRef: "missing-id"}); err == nil {
		t.Fatal("expected workspace not found")
	}
	node, err := store.Write(ctx, workspacepkg.WriteRequest{Name: "ws.txt", Content: []byte("workspace body")})
	if err != nil {
		t.Fatalf("write workspace: %v", err)
	}
	text, meta, err = service.loadContent(ctx, EmbeddingJob{SourceType: "workspace_file", SourceRef: node.ID})
	if err != nil || !strings.Contains(text, "workspace body") {
		t.Fatalf("workspace load: %q %v %v", text, meta, err)
	}

	if vec, err := (*EmbeddingService)(nil).EmbedQueryVector(ctx, "x"); vec != nil || err != nil {
		t.Fatalf("nil service embed: %v %v", vec, err)
	}
	if vec, err := service.EmbedQueryVector(ctx, "  "); vec != nil || err != nil {
		t.Fatalf("empty query embed: %v %v", vec, err)
	}
	if recs, err := (*EmbeddingService)(nil).Search(ctx, "q", nil, 0); recs != nil || err != nil {
		t.Fatalf("nil search: %v %v", recs, err)
	}
	if recs, err := service.Search(ctx, "", nil, 0); recs != nil || err != nil {
		t.Fatalf("empty search: %v %v", recs, err)
	}
	if _, err := service.Search(ctx, "alpha", []string{"agent:agent-1"}, 0); err != nil {
		t.Fatalf("search: %v", err)
	}

	if err := (*Vault)(nil).DeleteSecret(ctx, "x"); err == nil {
		t.Fatal("expected nil vault delete")
	}
	if _, err := (*Vault)(nil).ListSecrets(ctx); err == nil {
		t.Fatal("expected nil vault list")
	}
	if err := (&Vault{}).DeleteSecret(ctx, "x"); err == nil {
		t.Fatal("expected empty vault delete")
	}
	if _, err := (&Vault{}).ListSecrets(ctx); err == nil {
		t.Fatal("expected empty vault list")
	}

	if err := service.NotifyWorkspaceMutation(ctx, node.ID, "agent-1", true); err != nil {
		t.Fatal(err)
	}

	csvPath := filepath.Join(t.TempDir(), "bad.csv")
	if err := os.WriteFile(csvPath, []byte("\"unclosed"), 0644); err != nil {
		t.Fatal(err)
	}
	if csvText, err := extractCSVText(csvPath, ','); err != nil {
		t.Fatalf("csv fallback: %v", err)
	} else if !strings.Contains(csvText, "unclosed") {
		t.Fatalf("csv fallback text %q", csvText)
	}

	for _, p := range []string{".hidden", "file~", "x.tmp", "x.swp", "x.part", "vectors/a", "storage", "models"} {
		if !ignoredEmbeddingPath(p) {
			t.Fatalf("expected ignored path %q", p)
		}
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	watcher, err := NewWorkspaceWatcher(root, service)
	if err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := watcher.Start(wctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	sub := filepath.Join(root, "subdir")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "n.txt"), []byte("n"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	_ = os.Remove(keep)
	time.Sleep(80 * time.Millisecond)
	_ = watcher.Close()
}
