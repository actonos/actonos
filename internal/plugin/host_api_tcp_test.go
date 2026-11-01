package plugin

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/security"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

type tlsMaterial struct {
	certs []tls.Certificate
	roots *x509.CertPool
}

func testTLS(t *testing.T) tlsMaterial {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.NotFoundHandler())
	srv.StartTLS()
	defer srv.Close()
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	return tlsMaterial{certs: srv.TLS.Certificates, roots: roots}
}

// serve runs handle for every accepted connection and returns the listener's port.
func serve(t *testing.T, handle func(net.Conn)) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func echoUpper(c net.Conn) {
	defer c.Close()
	buf := make([]byte, 1024)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			_, _ = c.Write([]byte(strings.ToUpper(string(buf[:n]))))
		}
		if err != nil {
			return
		}
	}
}

type smtpOpts struct {
	offerSTARTTLS bool
	injectAfter   bool // sends plaintext right after "220 go ahead", before the handshake
}

func smtpServer(t *testing.T, m tlsMaterial, o smtpOpts) int {
	return serve(t, func(c net.Conn) {
		br := bufio.NewReader(c)
		fmt.Fprint(c, "220 mail.example.com ESMTP\r\n")
		if line, _ := br.ReadString('\n'); !strings.HasPrefix(line, "EHLO") {
			c.Close()
			return
		}
		if o.offerSTARTTLS {
			fmt.Fprint(c, "250-mail.example.com\r\n250-PIPELINING\r\n250 STARTTLS\r\n")
		} else {
			// What a downgrade attacker produces: the capability is stripped from the list, while
			// the server would still accept the command.
			fmt.Fprint(c, "250-mail.example.com\r\n250 PIPELINING\r\n")
		}
		if line, _ := br.ReadString('\n'); !strings.HasPrefix(line, "STARTTLS") {
			c.Close()
			return
		}
		if o.injectAfter {
			fmt.Fprint(c, "220 go ahead\r\n250 injected\r\n")
		} else {
			fmt.Fprint(c, "220 go ahead\r\n")
		}
		tc := tls.Server(c, &tls.Config{Certificates: m.certs})
		if tc.Handshake() != nil {
			c.Close()
			return
		}
		echoUpper(tc)
	})
}

func imapServer(t *testing.T, m tlsMaterial) int {
	return serve(t, func(c net.Conn) {
		br := bufio.NewReader(c)
		fmt.Fprint(c, "* OK IMAP ready\r\n")
		if line, _ := br.ReadString('\n'); !strings.HasPrefix(line, "a1 STARTTLS") {
			c.Close()
			return
		}
		fmt.Fprint(c, "a1 OK begin TLS\r\n")
		tc := tls.Server(c, &tls.Config{Certificates: m.certs})
		if tc.Handshake() != nil {
			c.Close()
			return
		}
		echoUpper(tc)
	})
}

type tcpEnv struct {
	ctx  context.Context
	host *HostContext
	mod  api.Module
}

func newTCPEnv(t *testing.T, perms PluginPermissions, m tlsMaterial) *tcpEnv {
	t.Helper()
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })
	mod, err := rt.Instantiate(ctx, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01})
	if err != nil {
		t.Fatal(err)
	}
	manifest := PluginManifest{ID: "imap", Permissions: perms}
	host := &HostContext{PluginID: "imap", Manifest: manifest, Gate: NewSecurityGate(manifest),
		Dial: security.DialPolicy{AllowLoopback: true, TLS: &tls.Config{RootCAs: m.roots},
			Resolve: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }}}
	t.Cleanup(host.CloseAllTCP)
	return &tcpEnv{ctx: WithHostContext(ctx, host), host: host, mod: mod}
}

func (e *tcpEnv) connect(t *testing.T, hostname string, port int, mode string) int32 {
	t.Helper()
	req, _ := json.Marshal(tcpConnectRequest{Host: hostname, Port: port, Mode: mode})
	e.mod.Memory().Write(64, req)
	return tcpConnect(e.ctx, e.mod, 64, uint32(len(req)))
}

func (e *tcpEnv) send(t *testing.T, h int32, s string) int32 {
	t.Helper()
	e.mod.Memory().Write(4096, []byte(s))
	return tcpSend(e.ctx, e.mod, h, 4096, uint32(len(s)))
}

func (e *tcpEnv) recv(h int32) (string, int32) {
	n := tcpRecv(e.ctx, e.mod, h, 1024, 2000)
	if n <= 0 {
		return "", n
	}
	return string(e.host.LastResponse), n
}

func perms(entries ...string) PluginPermissions { return PluginPermissions{NetTCP: entries} }

func TestImplicitTLSRoundTrip(t *testing.T) {
	m := testTLS(t)
	port := serve(t, func(c net.Conn) { echoUpper(tls.Server(c, &tls.Config{Certificates: m.certs})) })
	env := newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", port)), m)
	h := env.connect(t, "example.com", port, "tls")
	if h <= 0 {
		t.Fatalf("connect = %d", h)
	}
	if n := env.send(t, h, "hello"); n != 5 {
		t.Fatalf("send = %d", n)
	}
	if got, _ := env.recv(h); got != "HELLO" {
		t.Fatalf("recv = %q", got)
	}
	if tcpClose(env.ctx, env.mod, h) != 0 || env.send(t, h, "x") != TCPErrClosed {
		t.Fatal("a closed handle must stay closed")
	}
}

func TestSMTPStartTLSIsRunByTheHostAndEndsEncrypted(t *testing.T) {
	m := testTLS(t)
	port := smtpServer(t, m, smtpOpts{offerSTARTTLS: true})
	env := newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", port)), m)
	h := env.connect(t, "example.com", port, "smtp-starttls")
	if h <= 0 {
		t.Fatalf("connect = %d", h)
	}
	env.send(t, h, "ehlo again")
	if got, _ := env.recv(h); got != "EHLO AGAIN" {
		t.Fatalf("the upgraded channel should echo, got %q", got)
	}
}

func TestIMAPStartTLS(t *testing.T) {
	m := testTLS(t)
	port := imapServer(t, m)
	env := newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", port)), m)
	h := env.connect(t, "example.com", port, "imap-starttls")
	if h <= 0 {
		t.Fatalf("connect = %d", h)
	}
	env.send(t, h, "a2 capability")
	if got, _ := env.recv(h); got != "A2 CAPABILITY" {
		t.Fatalf("got %q", got)
	}
}

func TestStartTLSIsNeverDowngradedOrInjected(t *testing.T) {
	m := testTLS(t)
	noOffer := smtpServer(t, m, smtpOpts{offerSTARTTLS: false})
	inject := smtpServer(t, m, smtpOpts{offerSTARTTLS: true, injectAfter: true})
	env := newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", noOffer), fmt.Sprintf("example.com:%d", inject)), m)
	if h := env.connect(t, "example.com", noOffer, "smtp-starttls"); h != TCPErrConnect {
		t.Fatalf("a server that does not offer STARTTLS must be refused, got %d", h)
	}
	if h := env.connect(t, "example.com", inject, "smtp-starttls"); h != TCPErrConnect {
		t.Fatalf("plaintext injected after STARTTLS must abort the connection, got %d", h)
	}
	if len(env.host.tcpConns) != 0 {
		t.Fatal("failed connections must not occupy a slot")
	}
}

func TestOnlyTheListedHostAndPortAreReachable(t *testing.T) {
	m := testTLS(t)
	port := serve(t, func(c net.Conn) { echoUpper(tls.Server(c, &tls.Config{Certificates: m.certs})) })
	env := newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", port+1)), m) // wrong port
	if h := env.connect(t, "example.com", port, "tls"); h != TCPErrDenied {
		t.Fatalf("wrong port: %d", h)
	}
	env = newTCPEnv(t, perms(fmt.Sprintf("other.example:%d", port)), m)
	if h := env.connect(t, "example.com", port, "tls"); h != TCPErrDenied {
		t.Fatalf("wrong host: %d", h)
	}
	env = newTCPEnv(t, PluginPermissions{}, m)
	if h := env.connect(t, "example.com", port, "tls"); h != TCPErrDenied {
		t.Fatalf("no permission: %d", h)
	}
	env = newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", port)), m)
	if h := env.connect(t, "example.com", port, "plaintext"); h != TCPErrDenied {
		t.Fatalf("there is no plaintext mode: %d", h)
	}
}

func TestProductionPolicyBlocksLocalTargetsEvenIfListed(t *testing.T) {
	m := testTLS(t)
	env := newTCPEnv(t, perms("127.0.0.1:993", "10.0.0.5:993", "metadata.internal:993"), m)
	env.host.Dial = security.DialPolicy{} // the production zero value
	for _, host := range []string{"127.0.0.1", "10.0.0.5"} {
		if h := env.connect(t, host, 993, "tls"); h != TCPErrBlocked {
			t.Errorf("%s: want blocked, got %d", host, h)
		}
	}
	env.host.Dial = security.DialPolicy{Resolve: func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil // a listed name that resolves to the cloud metadata address
	}}
	if h := env.connect(t, "metadata.internal", 993, "tls"); h != TCPErrBlocked {
		t.Errorf("a listed name resolving inside must be blocked, got %d", h)
	}
}

func TestAtMostFourConnectionsAndSlotsAreReleased(t *testing.T) {
	m := testTLS(t)
	port := serve(t, func(c net.Conn) { echoUpper(tls.Server(c, &tls.Config{Certificates: m.certs})) })
	env := newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", port)), m)
	var handles []int32
	for i := 0; i < 4; i++ {
		h := env.connect(t, "example.com", port, "tls")
		if h <= 0 {
			t.Fatalf("connection %d: %d", i, h)
		}
		handles = append(handles, h)
	}
	if h := env.connect(t, "example.com", port, "tls"); h != TCPErrTooMany {
		t.Fatalf("fifth connection: %d", h)
	}
	tcpClose(env.ctx, env.mod, handles[0])
	if h := env.connect(t, "example.com", port, "tls"); h <= 0 {
		t.Fatalf("a freed slot must be reusable: %d", h)
	}
}

func TestEachDirectionHasAByteBudget(t *testing.T) {
	m := testTLS(t)
	port := serve(t, func(c net.Conn) { echoUpper(tls.Server(c, &tls.Config{Certificates: m.certs})) })
	env := newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", port)), m)
	env.host.TCPByteLimit = 10
	h := env.connect(t, "example.com", port, "tls")
	if env.send(t, h, "123456") != 6 {
		t.Fatal("within budget")
	}
	if got := env.send(t, h, "123456"); got != TCPErrOverLimit {
		t.Fatalf("over budget: %d", got)
	}
}

func TestRecvTimesOutWithoutData(t *testing.T) {
	m := testTLS(t)
	port := serve(t, func(c net.Conn) {
		defer c.Close() // without this the garbage collector may close the idle connection mid-test
		tc := tls.Server(c, &tls.Config{Certificates: m.certs})
		_ = tc.Handshake()
		time.Sleep(2 * time.Second)
	})
	env := newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", port)), m)
	h := env.connect(t, "example.com", port, "tls")
	if n := tcpRecv(env.ctx, env.mod, h, 100, 100); n != 0 {
		t.Fatalf("a quiet peer is a timeout (0), got %d", n)
	}
}

func TestClosingThePluginClosesItsSockets(t *testing.T) {
	m := testTLS(t)
	var mu sync.Mutex
	closed := false
	port := serve(t, func(c net.Conn) {
		defer c.Close()
		tc := tls.Server(c, &tls.Config{Certificates: m.certs})
		_ = tc.Handshake()
		buf := make([]byte, 1)
		_, err := tc.Read(buf)
		mu.Lock()
		closed = err != nil
		mu.Unlock()
	})
	env := newTCPEnv(t, perms(fmt.Sprintf("example.com:%d", port)), m)
	if h := env.connect(t, "example.com", port, "tls"); h <= 0 {
		t.Fatal(h)
	}
	env.host.CloseAllTCP()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := closed
		mu.Unlock()
		if c {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the server never saw the connection close")
}
