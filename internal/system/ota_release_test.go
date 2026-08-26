package system

import "testing"

func TestCanonicalVersionAndCompare(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain", raw: "1.0.1", want: "v1.0.1"},
		{name: "prefixed", raw: "v1.0.1", want: "v1.0.1"},
		{name: "build", raw: "1.0.1+build", want: "v1.0.1"},
		{name: "double v invalid", raw: "vv1.0.1", want: ""},
		{name: "dev", raw: "0.0.0-dev", want: "v0.0.0-dev"},
		{name: "four part", raw: "1.0.0.1", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalVersion(tt.raw); got != tt.want {
				t.Fatalf("CanonicalVersion(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
	if !VersionNewer("v1.0.1", "1.0.0") {
		t.Fatal("1.0.1 should be newer than 1.0.0")
	}
	if VersionNewer("1.0.0", "v1.0.1") {
		t.Fatal("1.0.0 should not be newer than 1.0.1")
	}
	if VersionNewer("1.0.1+build", "1.0.1") {
		t.Fatal("build metadata must not count as newer")
	}
	if VersionNewer("vv1.0.1", "1.0.0") {
		t.Fatal("double-v prefix is invalid and must not report newer")
	}
}

func TestReleaseAssetName(t *testing.T) {
	got := ReleaseAssetName("actond", "1.0.1", "linux", "amd64")
	if got != "actond_v1.0.1_x86_64" {
		t.Fatalf("linux amd64 = %q", got)
	}
	got = ReleaseAssetName("embeddingd", "v1.0.1", "windows", "amd64")
	if got != "embeddingd_v1.0.1_x86_64.exe" {
		t.Fatalf("windows embeddingd = %q", got)
	}
	got = ReleaseAssetName("actond", "1.0.1", "linux", "arm64")
	if got != "actond_v1.0.1_arm64" {
		t.Fatalf("linux arm64 = %q", got)
	}
}

func TestNormalizeSHA256AndSUMS(t *testing.T) {
	if got := NormalizeSHA256("SHA256:AbCd"); got != "abcd" {
		t.Fatalf("prefix strip = %q", got)
	}
	sums := ParseSHA256SUMS("deadbeef  actond_v1.0.1_x86_64\ncafe *embeddingd_v1.0.1_x86_64\n")
	if sums["actond_v1.0.1_x86_64"] != "deadbeef" {
		t.Fatalf("gnu two-space: %+v", sums)
	}
	if sums["embeddingd_v1.0.1_x86_64"] != "cafe" {
		t.Fatalf("binary star: %+v", sums)
	}
}
