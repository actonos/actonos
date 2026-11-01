package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
)

type fakeCreds struct {
	err        error
	gotPlugin  string
	gotAccount string
	calls      int
}

func (f *fakeCreds) Credentials(_ context.Context, pluginID, accountID string) (Credentials, error) {
	f.calls++
	f.gotPlugin, f.gotAccount = pluginID, accountID
	return Credentials{Username: "me@example.com", Password: "hunter2-app-password"}, f.err
}

func credHost(t *testing.T, perms PluginPermissions, b CredentialBroker) (*HostContext, func(string) int32) {
	t.Helper()
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })
	mod, err := rt.Instantiate(ctx, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01})
	if err != nil {
		t.Fatal(err)
	}
	manifest := PluginManifest{ID: "caldav", Permissions: perms}
	host := &HostContext{PluginID: "caldav", Manifest: manifest, Gate: NewSecurityGate(manifest), Credentials: b}
	exec := WithHostContext(ctx, host)
	return host, func(account string) int32 {
		mod.Memory().Write(64, []byte(account))
		return accountGetCredentials(exec, mod, 64, uint32(len(account)))
	}
}

func TestCredentialsAreHandedOnlyToThePluginsOwnAccountsWithPermission(t *testing.T) {
	b := &fakeCreds{}
	host, get := credHost(t, PluginPermissions{Credentials: true}, b)
	if n := get("acct_1"); n <= 0 {
		t.Fatalf("got %d", n)
	}
	if b.gotPlugin != "caldav" || b.gotAccount != "acct_1" {
		t.Fatalf("the plugin id must come from the host: %q %q", b.gotPlugin, b.gotAccount)
	}
	var out map[string]string
	if err := json.Unmarshal(host.LastResponse, &out); err != nil || out["password"] != "hunter2-app-password" || out["username"] != "me@example.com" {
		t.Fatalf("bad answer %s", host.LastResponse)
	}
	host.AppendLog("password is hunter2-app-password")
	if strings.Contains(strings.Join(host.GetLogs(), "\n"), "hunter2-app-password") {
		t.Fatal("the password must be redacted from plugin logs")
	}
}

func TestCredentialRefusals(t *testing.T) {
	host, get := credHost(t, PluginPermissions{}, &fakeCreds{})
	if got := get("a"); got != CredErrDenied || len(host.LastResponse) != 0 {
		t.Fatalf("no permission: %d", got)
	}
	b := &fakeCreds{err: ErrCredentialsForbidden}
	host, get = credHost(t, PluginPermissions{Credentials: true}, b)
	if got := get("a"); got != CredErrDenied || len(host.LastResponse) != 0 {
		t.Fatalf("other plugin's account: %d", got)
	}
	_, get = credHost(t, PluginPermissions{Credentials: true}, &fakeCreds{err: errors.New("vault locked")})
	if got := get("a"); got != CredErrUnavailable {
		t.Fatalf("broker failure: %d", got)
	}
	_, get = credHost(t, PluginPermissions{Credentials: true}, nil)
	if got := get("a"); got != CredErrUnavailable {
		t.Fatalf("no broker: %d", got)
	}
	nb := &fakeCreds{}
	_, get = credHost(t, PluginPermissions{}, nb)
	get("a")
	if nb.calls != 0 {
		t.Fatal("a plugin without the permission must not even wake the broker")
	}
}
