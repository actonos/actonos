package security

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolvePath(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolvePath(root, "nested/file.txt", true); err != nil {
		t.Fatalf("valid path rejected: %v", err)
	}
	for _, candidate := range []string{"../escape", filepath.Join(root, "absolute.txt")} {
		if _, err := ResolvePath(root, candidate, true); !errors.Is(err, ErrPathEscape) {
			t.Fatalf("expected path escape for %q, got %v", candidate, err)
		}
	}
}

func TestResolvePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ResolvePath(root, filepath.Join("outside", "secret.txt"), true); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("expected symlink escape rejection, got %v", err)
	}
}

func TestValidateOutboundURLRejectsPrivateTargets(t *testing.T) {
	for _, target := range []string{
		"http://127.0.0.1/admin",
		"http://localhost/",
		"http://169.254.169.254/latest/meta-data",
		"file:///etc/passwd",
		"https://user:pass@example.com/",
		"https:///",
	} {
		if err := ValidateOutboundURL(context.Background(), target); err == nil {
			t.Fatalf("expected unsafe URL rejection for %s", target)
		}
	}
}

func TestValidateOutboundURLAllowsPublicLiteral(t *testing.T) {
	if err := ValidateOutboundURL(context.Background(), "https://93.184.216.34/resource"); err != nil {
		t.Fatalf("public IP literal rejected: %v", err)
	}
}

func TestValidateOutboundURLResolution(t *testing.T) {
	originalLookup := lookupIP
	t.Cleanup(func() { lookupIP = originalLookup })

	lookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	if err := ValidateOutboundURL(context.Background(), "https://example.test/path"); err != nil {
		t.Fatalf("public target rejected: %v", err)
	}

	lookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.2")}}, nil
	}
	if err := ValidateOutboundURL(context.Background(), "https://private.test"); err == nil {
		t.Fatal("expected private DNS result rejection")
	}

	lookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return nil, errors.New("resolver unavailable")
	}
	if err := ValidateOutboundURL(context.Background(), "https://broken.test"); err == nil {
		t.Fatal("expected DNS error")
	}
}

func TestResolveExistingPath(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("ok"), 0600); err != nil {
		t.Fatalf("creating file: %v", err)
	}
	resolved, err := ResolvePath(root, "file.txt", false)
	if err != nil {
		t.Fatalf("resolving existing file: %v", err)
	}
	if filepath.Base(resolved) != "file.txt" {
		t.Fatalf("unexpected resolved path: %s", resolved)
	}
}

func TestResolvePathMissingAndCanonicalRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "missing", "workspace")
	resolved, err := ResolvePath(root, filepath.Join("deep", "file.txt"), true)
	if err != nil {
		t.Fatalf("resolving beneath missing root: %v", err)
	}
	if !filepath.IsAbs(resolved) || filepath.Base(resolved) != "file.txt" {
		t.Fatalf("unexpected resolved path: %s", resolved)
	}
	if _, err := ResolvePath(root, "missing.txt", false); err == nil {
		t.Fatal("expected missing existing path to fail")
	}
}

func TestResolvePathCanonicalizesSymlinkRoot(t *testing.T) {
	actual := t.TempDir()
	parent := t.TempDir()
	link := filepath.Join(parent, "workspace-link")
	if err := os.Symlink(actual, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	target, err := ResolvePath(link, "nested.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(target) != actual {
		t.Fatalf("root symlink was not canonicalized: %s", target)
	}
}

func TestNearestExistingParentFallsBackAtFilesystemRoot(t *testing.T) {
	missing := filepath.Join(string(filepath.Separator), "actonos-path-that-must-not-exist", "child")
	if got := nearestExistingParent(missing); got != string(filepath.Separator) {
		t.Fatalf("expected filesystem root as nearest parent, got %q", got)
	}
}

func TestCanonicalizePotentialPathUnavailableVolume(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("unavailable drive path is Windows-specific")
	}
	path := `Z:\actonos-volume-that-must-not-exist\workspace`
	if got := canonicalizePotentialPath(path); got != path {
		t.Fatalf("unavailable path should remain unchanged, got %q", got)
	}
}
