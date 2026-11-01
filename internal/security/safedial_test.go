package security

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
)

func resolver(ips ...string) func(context.Context, string) ([]net.IP, error) {
	return func(context.Context, string) ([]net.IP, error) {
		out := make([]net.IP, len(ips))
		for i, s := range ips {
			out[i] = net.ParseIP(s)
		}
		return out, nil
	}
}

func TestSafeAddrRefusesLocalAndPrivateTargets(t *testing.T) {
	for _, host := range []string{
		"localhost", "LOCALHOST.", "app.localhost", "127.0.0.1", "::1", "10.0.0.5", "172.16.3.4", "192.168.1.1",
		"169.254.169.254", "100.64.0.1", "0.0.0.0", "224.0.0.1", "255.255.255.255", "fe80::1", "fd00::1", "",
	} {
		if _, err := (DialPolicy{}).SafeAddr(context.Background(), host); err == nil {
			t.Errorf("%q must be refused", host)
		}
	}
	if ip, err := (DialPolicy{}).SafeAddr(context.Background(), "93.184.216.34"); err != nil || ip.String() != "93.184.216.34" {
		t.Errorf("a public literal is fine: %v %v", ip, err)
	}
}

func TestSafeAddrRefusesANameThatAlsoPointsInside(t *testing.T) {
	p := DialPolicy{Resolve: resolver("93.184.216.34", "10.0.0.7")}
	if _, err := p.SafeAddr(context.Background(), "mail.example.com"); !errors.Is(err, ErrUnsafeAddress) {
		t.Fatalf("a mixed answer must be refused: %v", err)
	}
	p = DialPolicy{Resolve: resolver()}
	if _, err := p.SafeAddr(context.Background(), "mail.example.com"); err == nil {
		t.Fatal("no addresses must be an error")
	}
}

// A resolver that answers differently on every call (the classic rebinding trick) cannot hurt
// because the name is resolved exactly once and that answer is the one that is dialed.
func TestTheNameIsResolvedOnceSoRebindingCannotSwapTheTarget(t *testing.T) {
	var calls atomic.Int32
	p := DialPolicy{Resolve: func(context.Context, string) ([]net.IP, error) {
		if calls.Add(1) == 1 {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil // what a second lookup would have returned
	}, ConnectTimeout: 300 * 1e6}
	_, err := p.DialTCP(context.Background(), "evil.example.com", 9) // port 9: nothing listens; we only count lookups
	if err == nil {
		t.Fatal("nothing should answer on the public address in this test")
	}
	if calls.Load() != 1 {
		t.Fatalf("the name was resolved %d times, want exactly 1", calls.Load())
	}
}

func startTLSServer(t *testing.T) (host string, port int, roots *x509.CertPool) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	srv.StartTLS()
	t.Cleanup(srv.Close)
	roots = x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	h, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(p)
	return h, n, roots
}

func TestLoopbackIsRefusedUnlessTheTestOverrideIsSet(t *testing.T) {
	_, port, roots := startTLSServer(t)
	if _, err := (DialPolicy{TLS: &tls.Config{RootCAs: roots}}).DialTLS(context.Background(), "127.0.0.1", port); !errors.Is(err, ErrUnsafeAddress) {
		t.Fatalf("production policy must refuse loopback: %v", err)
	}
}

func TestTLSVerifiesTheNameNotTheDialedIP(t *testing.T) {
	_, port, roots := startTLSServer(t) // httptest's certificate is valid for example.com, 127.0.0.1 and ::1
	good := DialPolicy{AllowLoopback: true, TLS: &tls.Config{RootCAs: roots}, Resolve: resolver("127.0.0.1")}
	conn, err := good.DialTLS(context.Background(), "example.com", port)
	if err != nil {
		t.Fatalf("a certificate valid for the name must be accepted: %v", err)
	}
	_ = conn.Close()

	bad := DialPolicy{AllowLoopback: true, TLS: &tls.Config{RootCAs: roots}, Resolve: resolver("127.0.0.1")}
	if _, err := bad.DialTLS(context.Background(), "attacker.test", port); err == nil {
		t.Fatal("a certificate that does not match the requested name must be rejected")
	}
	noTrust := DialPolicy{AllowLoopback: true, Resolve: resolver("127.0.0.1")}
	if _, err := noTrust.DialTLS(context.Background(), "example.com", port); err == nil {
		t.Fatal("an untrusted certificate authority must be rejected")
	}
}

func TestInvalidPortsAreRefused(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		if _, err := (DialPolicy{AllowLoopback: true}).DialTCP(context.Background(), "127.0.0.1", port); err == nil {
			t.Errorf("port %d must be refused", port)
		}
	}
}
