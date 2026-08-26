package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCheckUsesGitHubJSONNeverHTML(t *testing.T) {
	var htmlHits atomic.Int32
	var latestHits atomic.Int32
	payload := githubRelease{
		TagName: "v1.0.1",
		Assets: []githubAsset{
			{
				Name:               "actond_v1.0.1_x86_64.exe",
				BrowserDownloadURL: "https://example.invalid/actond",
				Digest:             "sha256:" + strings.Repeat("a", 64),
			},
			{
				Name:               "actond_v1.0.1_arm64",
				BrowserDownloadURL: "https://example.invalid/arm",
				Digest:             "sha256:" + strings.Repeat("b", 64),
			},
		},
	}
	body, _ := json.Marshal(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/latest"):
			latestHits.Add(1)
			if r.Header.Get("Accept") != githubAccept {
				t.Errorf("missing GitHub accept header")
			}
			if !strings.HasPrefix(r.Header.Get("User-Agent"), "ActonOS-Daemon") {
				t.Errorf("missing User-Agent")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case r.URL.Path == "/actonos/actonos/releases" || r.URL.Path == "/releases":
			htmlHits.Add(1)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>releases</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	eng := NewOTAEngine(t.TempDir())
	eng.SetAllowLoopback(true)
	eng.SetSkipRestart(true)
	eng.SetPlatform("windows", "amd64")
	eng.SetAPIURL(srv.URL + "/repos/actonos/actonos/releases/latest")
	eng.SetVersionMeta("1.0.0", "abc", "now")

	res := eng.Check(context.Background(), "1.0.0", true, false)
	if htmlHits.Load() != 0 {
		t.Fatalf("HTML /releases was requested %d times", htmlHits.Load())
	}
	if latestHits.Load() == 0 {
		t.Fatal("expected GET /releases/latest")
	}
	if res.ErrorCode != "" {
		t.Fatalf("unexpected error %s %s", res.ErrorCode, res.ErrorMessage)
	}
	if !res.UpdateAvailable {
		t.Fatalf("expected update_available, got %+v", res)
	}
	if res.LatestVersion != "1.0.1" {
		t.Fatalf("latest = %q", res.LatestVersion)
	}
	// Foreign-arch linux/arm64 asset must not gate Windows host.
	if !res.UpdateAvailable {
		t.Fatal("foreign-arch assets must be ignored")
	}
}

func TestCheckRateLimitIsNotUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	}))
	t.Cleanup(srv.Close)

	eng := NewOTAEngine(t.TempDir())
	eng.SetAllowLoopback(true)
	eng.SetPlatform("linux", "amd64")
	eng.SetAPIURL(srv.URL + "/repos/actonos/actonos/releases/latest")

	res := eng.Check(context.Background(), "1.0.0", true, false)
	if res.UpdateAvailable {
		t.Fatal("429 must not set update_available")
	}
	if res.ErrorCode != ErrCodeRateLimit {
		t.Fatalf("error_code = %q", res.ErrorCode)
	}
	if res.RetryAfter != 42 {
		t.Fatalf("retry_after = %d", res.RetryAfter)
	}
}

func TestCheckMissingChecksumStillAvailable(t *testing.T) {
	payload := githubRelease{
		TagName: "v1.0.1",
		Assets: []githubAsset{
			{Name: "actond_v1.0.1_x86_64", BrowserDownloadURL: "https://example.invalid/a"},
		},
	}
	body, _ := json.Marshal(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	eng := NewOTAEngine(t.TempDir())
	eng.SetAllowLoopback(true)
	eng.SetPlatform("linux", "amd64")
	eng.SetAPIURL(srv.URL)

	res := eng.Check(context.Background(), "1.0.0", true, false)
	if !res.UpdateAvailable {
		t.Fatalf("missing digest must not clear update_available: %+v", res)
	}
	if !res.ChecksumMissing {
		t.Fatal("expected checksum_missing")
	}
	if res.CanInstall {
		t.Fatal("Install must be blocked without checksum")
	}
}

func TestCheckEmbeddingdRequiredMissingAsset(t *testing.T) {
	payload := githubRelease{
		TagName: "v1.0.1",
		Assets: []githubAsset{
			{
				Name:               "actond_v1.0.1_x86_64",
				BrowserDownloadURL: "https://example.invalid/a",
				Digest:             "sha256:" + strings.Repeat("a", 64),
			},
		},
	}
	body, _ := json.Marshal(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	eng := NewOTAEngine(t.TempDir())
	eng.SetAllowLoopback(true)
	eng.SetPlatform("linux", "amd64")
	eng.SetAPIURL(srv.URL)

	res := eng.Check(context.Background(), "1.0.0", true, true)
	if res.UpdateAvailable {
		t.Fatal("missing required embeddingd must clear update_available")
	}
	if res.ErrorCode != ErrCodeAssetMissing {
		t.Fatalf("error_code = %q", res.ErrorCode)
	}
}

func shaHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestCheckDevCurrentIsEligible(t *testing.T) {
	bin := []byte("actond-bytes")
	payload := githubRelease{
		TagName: "v1.0.1",
		Assets: []githubAsset{
			{
				Name:               "actond_v1.0.1_x86_64",
				BrowserDownloadURL: "placeholder",
				Digest:             "sha256:" + shaHex(bin),
			},
		},
	}
	_ = fmt.Sprintf("%s", payload.TagName)
	body, _ := json.Marshal(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	eng := NewOTAEngine(t.TempDir())
	eng.SetAllowLoopback(true)
	eng.SetPlatform("linux", "amd64")
	eng.SetAPIURL(srv.URL)
	res := eng.Check(context.Background(), "0.0.0-dev", true, false)
	if !res.UpdateAvailable {
		t.Fatalf("dev build should see 1.0.1 as available: %+v", res)
	}
}
