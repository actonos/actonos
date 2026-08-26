package system

import (
	"bufio"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"
)

// CanonicalVersion returns golang.org/x/mod/semver Canonical form (leading "v").
// A single leading v/V is stripped before Canonical("v"+raw) so Compare must
// use the result as-is — never "v"+canonical (that yields "vv1.0.1", which
// compares equal and hides every real update).
func CanonicalVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	raw = strings.TrimPrefix(raw, "V")
	if raw == "" {
		return ""
	}
	return semver.Canonical("v" + raw)
}

// VersionNewer reports whether latest is a SemVer greater than current.
// Both sides are Canonicalized; invalid versions are never newer.
func VersionNewer(latest, current string) bool {
	cl := CanonicalVersion(latest)
	cc := CanonicalVersion(current)
	if cl == "" || cc == "" {
		return false
	}
	return semver.Compare(cl, cc) > 0
}

// ArchLabel maps GOARCH to the ActonOS release asset arch token.
func ArchLabel(goarch string) string {
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "arm64"
	default:
		return goarch
	}
}

// ReleaseAssetName is {actond|embeddingd}_v{version}_{x86_64|arm64}[.exe].
func ReleaseAssetName(binary, version, goos, goarch string) string {
	canon := CanonicalVersion(version)
	ver := strings.TrimPrefix(canon, "v")
	if ver == "" {
		ver = strings.TrimPrefix(strings.TrimSpace(version), "v")
	}
	name := binary + "_v" + ver + "_" + ArchLabel(goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// HostBinaryFileName is the on-disk name under releases/{version}/ and bin/.
func HostBinaryFileName(binary, goos string) string {
	if goos == "windows" {
		return binary + ".exe"
	}
	return binary
}

// NormalizeSHA256 strips a case-insensitive "sha256:" prefix and lowercases hex.
func NormalizeSHA256(raw string) string {
	s := strings.TrimSpace(raw)
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "sha256:") {
		s = s[len("sha256:"):]
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// ParseSHA256SUMS parses GNU sha256sum two-field lines into name → hex.
func ParseSHA256SUMS(content string) map[string]string {
	out := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		hex, name, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		name = strings.TrimPrefix(name, "*")
		name = strings.TrimSpace(name)
		hex = NormalizeSHA256(hex)
		if hex == "" || name == "" {
			continue
		}
		out[name] = hex
	}
	return out
}
