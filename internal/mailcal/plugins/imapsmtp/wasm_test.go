package imapsmtp_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/refplugin/refplugintest"
	"github.com/actonos/actonos/internal/plugin"
	"github.com/actonos/actonos/internal/security"
)

type creds struct{}

func (creds) Credentials(_ context.Context, pluginID, accountID string) (plugin.Credentials, error) {
	return plugin.Credentials{Username: user, Password: pass}, nil
}

func wasmProvider(t *testing.T, e *env, netTCP []string) mailcal.Provider {
	t.Helper()
	raw, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m plugin.PluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Permissions.NetTCP = netTCP // what a user would add for their own server
	host := &plugin.HostContext{PluginID: m.ID, Manifest: m, Gate: plugin.NewSecurityGate(m), Credentials: creds{},
		Dial: security.DialPolicy{AllowLoopback: true, TLS: &tls.Config{RootCAs: e.roots},
			Resolve: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }}}
	inst := refplugintest.InstanceOf(t, refplugintest.BuildGuest(t, "./internal/mailcal/plugins/imapsmtp/wasm"), m, host)
	return mailcal.NewBridgeProvider(inst)
}

func starttlsAccount(e *env) abi.AccountRef {
	a := e.account()
	a.Config["imap_mode"], a.Config["smtp_mode"] = "starttls", "starttls"
	return a
}

// The compiled plugin, a stock IMAP client and a stock SMTP client, the host's sockets with the
// host-run STARTTLS in between, against real (in-process) servers.
func TestTheWasmPluginReadsAndSendsOverHostRunSTARTTLS(t *testing.T) {
	e := newEnvMode(t, true)
	e.seed("INBOX", m1)
	e.seed("Sent", m3, imap.FlagSeen)
	p := wasmProvider(t, e, []string{fmt.Sprintf("example.com:%d", e.imapPort), fmt.Sprintf("example.com:%d", e.smtpPort)})
	acct := starttlsAccount(e)
	ctx := context.Background()

	list, err := p.MailList(ctx, acct, abi.MailListParams{})
	if err != nil || len(list.Messages) != 1 || list.Messages[0].Subject != "Q3 invoice" {
		t.Fatalf("read over STARTTLS: %+v %v", list, err)
	}
	res, err := p.MailSend(ctx, acct, abi.MailSendParams{To: []abi.Address{{Email: "alice@partner.com"}}, Subject: "Re: Q3 invoice",
		BodyText: "Received, thank you.", IdempotencyKey: "idem_wasm"})
	if err != nil || res.Replayed {
		t.Fatalf("send over STARTTLS: %+v %v", res, err)
	}
	if e.deliveredCount() != 1 || !strings.Contains(e.delivered[0].data, "Received, thank you.") {
		t.Fatalf("delivered %d", e.deliveredCount())
	}
	again, err := p.MailSend(ctx, acct, abi.MailSendParams{To: []abi.Address{{Email: "alice@partner.com"}}, Subject: "Re: Q3 invoice",
		BodyText: "Received, thank you.", IdempotencyKey: "idem_wasm"})
	if err != nil || !again.Replayed || e.deliveredCount() != 1 {
		t.Fatalf("replay: %+v %v delivered=%d", again, err, e.deliveredCount())
	}
}

func TestTheWasmPluginCannotReachServersTheManifestDoesNotList(t *testing.T) {
	e := newEnvMode(t, true)
	p := wasmProvider(t, e, []string{fmt.Sprintf("example.com:%d", e.smtpPort)}) // IMAP not listed
	_, err := p.MailList(context.Background(), starttlsAccount(e), abi.MailListParams{})
	if !mailcal.IsCode(err, abi.ErrTransient) || !strings.Contains(err.Error(), "not permitted by the plugin manifest") {
		t.Fatalf("got %v", err)
	}
	var none mailcal.Provider = wasmProvider(t, e, nil)
	if _, err := none.MailList(context.Background(), starttlsAccount(e), abi.MailListParams{}); err == nil {
		t.Fatal("the shipped manifest has an empty net_tcp, so nothing is reachable until the user adds their server")
	}
}
