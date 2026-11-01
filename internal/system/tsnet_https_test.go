package system

import "testing"

func TestHTTPSURLUsesTheFirstUsableCertificateDomain(t *testing.T) {
	cases := []struct {
		name    string
		domains []string
		want    string
	}{
		{"none means HTTPS is not enabled for the tailnet", nil, ""},
		{"trailing dot is dropped", []string{"acton-mini.tail1234.ts.net."}, "https://acton-mini.tail1234.ts.net"},
		{"wildcards and blanks are skipped", []string{"", "*.tail1234.ts.net", "box.tail1234.ts.net"}, "https://box.tail1234.ts.net"},
	}
	for _, c := range cases {
		if got := httpsURLFor(c.domains); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTailscaleHTTPSIsOnUnlessSwitchedOff(t *testing.T) {
	for value, want := range map[string]bool{"": true, "1": true, "true": true, "0": false, "false": false, "OFF": false, " no ": false} {
		t.Setenv("ACTON_TAILSCALE_HTTPS", value)
		if got := TailscaleHTTPSEnabled(); got != want {
			t.Errorf("ACTON_TAILSCALE_HTTPS=%q: got %v, want %v", value, got, want)
		}
	}
}
