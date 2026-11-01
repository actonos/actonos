package mailcal_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/refplugin/refplugintest"
	"github.com/actonos/actonos/internal/plugin"
	"github.com/actonos/actonos/internal/security"
)

// fakeIMAP speaks just enough IMAP4rev1: STARTTLS, then LOGIN, CAPABILITY, SELECT, NOOP, LOGOUT.
func fakeIMAP(t *testing.T, exists int, goodPassword string) (port int, roots *x509.CertPool) {
	t.Helper()
	ts := httptest.NewUnstartedServer(http.NotFoundHandler())
	ts.StartTLS()
	certs := ts.TLS.Certificates
	roots = x509.NewCertPool()
	roots.AddCert(ts.Certificate())
	ts.Close()

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
			go func() {
				defer c.Close()
				br := bufio.NewReader(c)
				fmt.Fprint(c, "* OK IMAP ready\r\n")
				if line, _ := br.ReadString('\n'); !strings.HasPrefix(line, "a1 STARTTLS") {
					return
				}
				fmt.Fprint(c, "a1 OK begin TLS\r\n")
				tc := tls.Server(c, &tls.Config{Certificates: certs})
				if tc.Handshake() != nil {
					return
				}
				r := bufio.NewReader(tc)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					f := strings.Fields(strings.TrimSpace(line))
					if len(f) < 2 {
						continue
					}
					tag, cmd := f[0], strings.ToUpper(f[1])
					switch cmd {
					case "CAPABILITY":
						fmt.Fprintf(tc, "* CAPABILITY IMAP4rev1 AUTH=PLAIN\r\n%s OK done\r\n", tag)
					case "LOGIN":
						if len(f) >= 4 && strings.Trim(f[3], `"`) == goodPassword {
							fmt.Fprintf(tc, "%s OK LOGIN completed\r\n", tag)
						} else {
							fmt.Fprintf(tc, "%s NO [AUTHENTICATIONFAILED] bad credentials\r\n", tag)
						}
					case "SELECT":
						fmt.Fprintf(tc, "* %d EXISTS\r\n* 0 RECENT\r\n* OK [UIDVALIDITY 1] ok\r\n* FLAGS (\\Seen)\r\n%s OK [READ-WRITE] SELECT completed\r\n", exists, tag)
					case "NOOP":
						fmt.Fprintf(tc, "%s OK noop\r\n", tag)
					case "LOGOUT":
						fmt.Fprintf(tc, "* BYE bye\r\n%s OK LOGOUT completed\r\n", tag)
						return
					default:
						fmt.Fprintf(tc, "%s BAD unsupported\r\n", tag)
					}
				}
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, roots
}

func imapProbe(t *testing.T, port int, roots *x509.CertPool, perms plugin.PluginPermissions) mailcal.Provider {
	t.Helper()
	m := plugin.PluginManifest{ID: "imapprobe", Name: "probe", Version: "1", Capabilities: []string{"mail@1"}, Permissions: perms}
	host := &plugin.HostContext{PluginID: m.ID, Manifest: m, Gate: plugin.NewSecurityGate(m),
		Dial: security.DialPolicy{AllowLoopback: true, TLS: &tls.Config{RootCAs: roots},
			Resolve: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }}}
	inst := refplugintest.InstanceOf(t, refplugintest.BuildGuest(t, "./internal/mailcal/refplugin/imapguest"), m, host)
	return mailcal.NewBridgeProvider(inst)
}

func TestAStockIMAPClientRunsInsideWebAssemblyOverTheHostsTLSSockets(t *testing.T) {
	port, roots := fakeIMAP(t, 7, "secret")
	p := imapProbe(t, port, roots, plugin.PluginPermissions{NetTCP: []string{fmt.Sprintf("example.com:%d", port)}})
	start := time.Now()
	res, err := p.MailList(context.Background(), abi.AccountRef{ID: "a", Config: map[string]any{"host": "example.com", "port": float64(port)}}, abi.MailListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].Subject != "7 messages" {
		t.Fatalf("unexpected result %+v", res.Messages)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("the polling reader is too slow: %s", d)
	}
}

func TestTheGuestCannotConnectWhereTheManifestDoesNotAllow(t *testing.T) {
	port, roots := fakeIMAP(t, 1, "secret")
	p := imapProbe(t, port, roots, plugin.PluginPermissions{NetTCP: []string{fmt.Sprintf("example.com:%d", port+1)}})
	_, err := p.MailList(context.Background(), abi.AccountRef{ID: "a", Config: map[string]any{"host": "example.com", "port": float64(port)}}, abi.MailListParams{})
	if !mailcal.IsCode(err, abi.ErrTransient) || !strings.Contains(err.Error(), "not permitted by the plugin manifest") {
		t.Fatalf("got %v", err)
	}
}
