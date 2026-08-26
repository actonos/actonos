package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingRestarter struct {
	mu      sync.Mutex
	daemons int
	embeds  int
}

func (r *recordingRestarter) RestartEmbeddingd(context.Context) error {
	r.mu.Lock()
	r.embeds++
	r.mu.Unlock()
	return nil
}
func (r *recordingRestarter) RestartDaemon(context.Context) error {
	r.mu.Lock()
	r.daemons++
	r.mu.Unlock()
	return nil
}

func shaOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestApplyDownloadsBothBinariesAndActivates(t *testing.T) {
	if ok, reason := OTAApplySupport(); !ok {
		t.Skip("apply unsupported: " + reason)
	}
	actond := []byte("actond-binary-v101")
	emb := []byte("embeddingd-binary-v101")
	goos, goarch := runtime.GOOS, runtime.GOARCH
	actondName := ReleaseAssetName("actond", "1.0.1", goos, goarch)
	embName := ReleaseAssetName("embeddingd", "1.0.1", goos, goarch)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			rel := githubRelease{
				TagName: "v1.0.1",
				Assets: []githubAsset{
					{Name: actondName, BrowserDownloadURL: srv.URL + "/dl/" + actondName, Digest: "sha256:" + shaOf(actond)},
					{Name: embName, BrowserDownloadURL: srv.URL + "/dl/" + embName, Digest: "sha256:" + shaOf(emb)},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		case strings.Contains(r.URL.Path, "actond"):
			_, _ = w.Write(actond)
		case strings.Contains(r.URL.Path, "embeddingd"):
			_, _ = w.Write(emb)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	eng := NewOTAEngine(dir)
	eng.SetAllowLoopback(true)
	eng.SetSkipRestart(true)
	eng.SetRestarter(&recordingRestarter{})
	eng.SetPlatform(goos, goarch)
	eng.SetAPIURL(srv.URL + "/repos/actonos/actonos/releases/latest")

	res := eng.Check(context.Background(), "1.0.0", true, true)
	if !res.UpdateAvailable {
		t.Fatalf("check: %+v", res)
	}
	if err := eng.EnqueueApply(context.Background(), "1.0.0", true); err != nil {
		t.Fatal(err)
	}
	waitJobTerminal(t, eng, 5*time.Second)

	verDir := filepath.Join(dir, "releases", "1.0.1")
	if _, err := os.Stat(filepath.Join(verDir, HostBinaryFileName("actond", goos))); err != nil {
		t.Fatalf("actond not in releases/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(verDir, HostBinaryFileName("embeddingd", goos))); err != nil {
		t.Fatalf("embeddingd not in releases/: %v", err)
	}
	binActond := filepath.Join(dir, "bin", HostBinaryFileName("actond", goos))
	if _, err := os.Stat(binActond); err != nil {
		t.Fatalf("actond not activated into bin/: %v", err)
	}
}

func TestApplyChecksumMismatchRefusesActivate(t *testing.T) {
	if ok, reason := OTAApplySupport(); !ok {
		t.Skip("apply unsupported: " + reason)
	}
	actond := []byte("good-bytes")
	goos, goarch := runtime.GOOS, runtime.GOARCH
	name := ReleaseAssetName("actond", "1.0.1", goos, goarch)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v1.0.1",
				Assets: []githubAsset{{
					Name: name, BrowserDownloadURL: srv.URL + "/dl/" + name,
					Digest: "sha256:" + strings.Repeat("0", 64),
				}},
			})
			return
		}
		_, _ = w.Write(actond)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	eng := NewOTAEngine(dir)
	eng.SetAllowLoopback(true)
	eng.SetSkipRestart(true)
	eng.SetPlatform(goos, goarch)
	eng.SetAPIURL(srv.URL + "/repos/actonos/actonos/releases/latest")
	_ = eng.Check(context.Background(), "1.0.0", true, false)
	if err := eng.EnqueueApply(context.Background(), "1.0.0", false); err != nil {
		t.Fatal(err)
	}
	job := waitJobTerminal(t, eng, 5*time.Second)
	if job.Status != JobFailed {
		t.Fatalf("status = %s want failed", job.Status)
	}
	if _, err := os.Stat(filepath.Join(dir, "bin", HostBinaryFileName("actond", goos))); err == nil {
		t.Fatal("checksum mismatch must not activate into bin/")
	}
}

func TestEnqueueReturnsBeforeSlowDownload(t *testing.T) {
	if ok, reason := OTAApplySupport(); !ok {
		t.Skip("apply unsupported: " + reason)
	}
	goos, goarch := runtime.GOOS, runtime.GOARCH
	name := ReleaseAssetName("actond", "1.0.1", goos, goarch)
	payload := []byte("slow-actond")
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "v1.0.1",
				Assets: []githubAsset{{
					Name: name, BrowserDownloadURL: srv.URL + "/dl/" + name,
					Digest: "sha256:" + shaOf(payload),
				}},
			})
			return
		}
		time.Sleep(400 * time.Millisecond)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	eng := NewOTAEngine(t.TempDir())
	eng.SetAllowLoopback(true)
	eng.SetSkipRestart(true)
	eng.SetPlatform(goos, goarch)
	eng.SetAPIURL(srv.URL + "/repos/actonos/actonos/releases/latest")
	_ = eng.Check(context.Background(), "1.0.0", true, false)

	started := time.Now()
	if err := eng.EnqueueApply(context.Background(), "1.0.0", false); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 150*time.Millisecond {
		t.Fatalf("enqueue blocked for %s", time.Since(started))
	}
	_ = waitJobTerminal(t, eng, 5*time.Second)
}

func TestLeftoverApplyingJobReclaimed(t *testing.T) {
	dir := t.TempDir()
	eng := NewOTAEngine(dir)
	eng.SetSkipRestart(true)
	job := &OTAJob{ID: "ota_1", Action: "apply", Status: JobDownloading, Version: "1.0.1", StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	eng.mu.Lock()
	eng.setJobLocked(job)
	eng.mu.Unlock()

	eng2 := NewOTAEngine(dir)
	got := eng2.Job()
	if got == nil || got.Status != JobInterrupted {
		t.Fatalf("reclaimed job = %+v", got)
	}
}

func TestEnqueueAllowedAfterFailed(t *testing.T) {
	dir := t.TempDir()
	eng := NewOTAEngine(dir)
	eng.SetSkipRestart(true)
	eng.mu.Lock()
	eng.setJobLocked(&OTAJob{ID: "x", Action: "apply", Status: JobFailed, UpdatedAt: time.Now().UTC(), StartedAt: time.Now().UTC()})
	eng.mu.Unlock()
	if JobIsActive(eng.Job().Status) {
		t.Fatal("failed is terminal")
	}
}

func TestRollbackRefusedWhileActive(t *testing.T) {
	eng := NewOTAEngine(t.TempDir())
	eng.SetSkipRestart(true)
	eng.mu.Lock()
	eng.previousBuild = filepath.Join(t.TempDir(), "old")
	eng.setJobLocked(&OTAJob{ID: "x", Action: "apply", Status: JobDownloading, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	eng.mu.Unlock()
	if err := eng.EnqueueRollback(); err != ErrOTAJobActive {
		t.Fatalf("err = %v", err)
	}
}

func waitJobTerminal(t *testing.T, eng *OTAEngine, d time.Duration) *OTAJob {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		job := eng.Job()
		if job == nil || JobIsTerminal(job.Status) {
			return job
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job did not terminate: %+v", eng.Job())
	return eng.Job()
}
