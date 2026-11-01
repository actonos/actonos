package security

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathMissingTargetRequiresAllowMissing(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolvePath(root, "does-not-exist.txt", false); err == nil {
		t.Fatal("expected error resolving a missing file when allowMissing=false")
	}
	if _, err := ResolvePath(root, "does-not-exist.txt", true); err != nil {
		t.Fatalf("missing file must be allowed when allowMissing=true: %v", err)
	}
}

func TestResolvePathRejectsExistingSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Existing target, allowMissing=false: the resolved path leaves the root.
	if _, err := ResolvePath(root, "link.txt", false); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("expected file-symlink escape rejection, got %v", err)
	}
}

func TestResolvePathAllowsSymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := ResolvePath(root, filepath.Join("alias", "new.txt"), true)
	if err != nil {
		t.Fatalf("in-root symlink rejected: %v", err)
	}
	if filepath.Base(filepath.Dir(got)) != "real" {
		t.Fatalf("symlink not resolved to its real location: %s", got)
	}
}

func TestResolvePathWithBaseRejectsAbsoluteAndSiblingPrefix(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "data")
	sibling := filepath.Join(parent, "data-evil")
	for _, d := range []string{root, sibling} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ResolvePathWithBase(root, root, "/etc/passwd", true); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("absolute path must be rejected, got %v", err)
	}
	// "../data-evil" shares a string prefix with the root but is outside it.
	if _, err := ResolvePathWithBase(root, root, filepath.Join("..", "data-evil", "f"), true); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("sibling-prefix directory must be rejected, got %v", err)
	}
}

func TestNearestExistingParent(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if got := nearestExistingParent(deep); got != root {
		t.Fatalf("nearestExistingParent(%s) = %s, want %s", deep, got, root)
	}
	if got := nearestExistingParent(root); got != root {
		t.Fatalf("existing path must be returned as-is, got %s", got)
	}
}

func TestCanonicalizePotentialPath(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	want, _ := filepath.EvalSymlinks(real)

	if got := canonicalizePotentialPath(link); got != want {
		t.Fatalf("existing symlink: got %s, want %s", got, want)
	}
	if got := canonicalizePotentialPath(filepath.Join(link, "x", "y")); got != filepath.Join(want, "x", "y") {
		t.Fatalf("missing tail beneath symlink: got %s", got)
	}
}

func TestValidateOutboundURLEdgeCases(t *testing.T) {
	originalLookup := lookupIP
	t.Cleanup(func() { lookupIP = originalLookup })
	lookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}

	for _, target := range []string{
		"http://[::1",              // unparsable
		"http://0.0.0.0/",          // unspecified
		"http://[::1]/",            // IPv6 loopback
		"http://[fc00::1]/",        // IPv6 unique local
		"http://[fe80::1]/",        // IPv6 link-local
		"http://192.168.1.10/",     // RFC1918
		"http://172.16.0.1/",       // RFC1918
		"http://app.localhost/",    // localhost subdomain
		"http://LOCALHOST./",       // trailing dot / case
		"ftp://example.com/file",   // unsupported scheme
		"gopher://example.com/",    // unsupported scheme
		"http://user@example.com/", // credentials without password
	} {
		if err := ValidateOutboundURL(context.Background(), target); err == nil {
			t.Errorf("expected rejection for %s", target)
		}
	}

	for _, target := range []string{
		"https://example.com/path?q=1",
		"wss://example.com/socket",
		"http://93.184.216.34:8080/",
	} {
		if err := ValidateOutboundURL(context.Background(), target); err != nil {
			t.Errorf("expected %s to be allowed: %v", target, err)
		}
	}
}

func TestValidateOutboundURLRejectsMixedDNSAnswers(t *testing.T) {
	originalLookup := lookupIP
	t.Cleanup(func() { lookupIP = originalLookup })
	// One public and one private answer: a rebinding-style response must be refused.
	lookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if err := ValidateOutboundURL(context.Background(), "https://rebind.test/"); !errors.Is(err, ErrUnsafeURL) {
		t.Fatalf("expected ErrUnsafeURL, got %v", err)
	}
}

// A symlink whose target does not exist yet is still a symlink: writing through
// it would create the file at the target, outside the root.
func TestResolvePathRejectsDanglingSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "not-created-yet.txt")
	if err := os.Symlink(target, filepath.Join(root, "dangling")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := ResolvePath(root, "dangling", true)
	if !errors.Is(err, ErrPathEscape) {
		t.Errorf("allowMissing=true: dangling symlink to %s must be rejected, got path=%q err=%v", target, got, err)
	}
	// Writing through a link in the middle of the path escapes just the same.
	got, err = ResolvePath(root, filepath.Join("dangling", "nested", "file.txt"), true)
	if !errors.Is(err, ErrPathEscape) {
		t.Errorf("nested path beneath a dangling symlink must be rejected, got path=%q err=%v", got, err)
	}
	if _, err = ResolvePath(root, "dangling", false); err == nil {
		t.Error("allowMissing=false must reject a dangling symlink")
	}
}

func TestResolvePathSymlinkLoopTerminates(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	if err := os.Symlink(b, a); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}
	// Must return (an error or an in-root path), never hang or panic.
	_, _ = ResolvePath(root, "a", true)
	_, _ = ResolvePath(root, "a", false)
}

func TestResolvePathAllowsDanglingSymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "later.txt"), filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ResolvePath(root, "alias", true); err != nil {
		t.Fatalf("a dangling symlink that stays inside the root is harmless, got %v", err)
	}
}

func TestFollowDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	want, _ := filepath.EvalSymlinks(root)

	if _, ok := followDanglingSymlink(filepath.Join(root, "missing")); ok {
		t.Fatal("a nonexistent path is not a symlink")
	}
	regular := filepath.Join(root, "regular")
	if err := os.WriteFile(regular, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := followDanglingSymlink(regular); ok {
		t.Fatal("a regular file is not a symlink")
	}

	// Relative target resolves against the link's own directory.
	if err := os.Symlink("later.txt", filepath.Join(root, "relative")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, ok := followDanglingSymlink(filepath.Join(root, "relative"))
	if !ok || got != filepath.Join(want, "later.txt") {
		t.Fatalf("relative link resolved to %q (ok=%v), want %q", got, ok, filepath.Join(want, "later.txt"))
	}

	// A chain of links is followed to its final, still-missing target.
	if err := os.Symlink(filepath.Join(root, "relative"), filepath.Join(root, "hop2")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "hop2"), filepath.Join(root, "hop1")); err != nil {
		t.Fatal(err)
	}
	got, ok = followDanglingSymlink(filepath.Join(root, "hop1"))
	if !ok || got != filepath.Join(want, "later.txt") {
		t.Fatalf("chained links resolved to %q (ok=%v), want %q", got, ok, filepath.Join(want, "later.txt"))
	}

	// A loop is cut off after a bounded number of hops instead of spinning.
	if err := os.Symlink(filepath.Join(root, "loop-b"), filepath.Join(root, "loop-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "loop-a"), filepath.Join(root, "loop-b")); err != nil {
		t.Fatal(err)
	}
	if _, ok := followDanglingSymlink(filepath.Join(root, "loop-a")); !ok {
		t.Fatal("a loop is still a symlink; it must terminate and report ok")
	}
}
