package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
)

type fakeBroker struct {
	err     error
	grant   TokenGrant
	gotPlug string
	gotAcct string
	calls   int
}

func (f *fakeBroker) AccessToken(_ context.Context, pluginID, accountID string) (TokenGrant, error) {
	f.calls++
	f.gotPlug, f.gotAcct = pluginID, accountID
	return f.grant, f.err
}

func oauthHost(t *testing.T, perms PluginPermissions, broker TokenBroker) (context.Context, *HostContext, func(string) int32) {
	t.Helper()
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = rt.Close(ctx) })
	mod, err := rt.Instantiate(ctx, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01})
	if err != nil {
		t.Fatal(err)
	}
	manifest := PluginManifest{ID: "gmail", Permissions: perms}
	host := &HostContext{PluginID: "gmail", Manifest: manifest, Gate: NewSecurityGate(manifest), Tokens: broker}
	exec := WithHostContext(ctx, host)
	return exec, host, func(account string) int32 {
		if !mod.Memory().Write(64, []byte(account)) {
			t.Fatal("write")
		}
		return oauthGetToken(exec, mod, 64, uint32(len(account)))
	}
}

func TestOAuthGetTokenHandsOutOnlyTheAccessToken(t *testing.T) {
	b := &fakeBroker{grant: TokenGrant{Provider: "google", AccessToken: "ya29.secret", ExpiresAt: time.Now().Add(time.Hour)}}
	_, host, get := oauthHost(t, PluginPermissions{OAuth: []string{"google"}}, b)
	n := get("acct_1")
	if n <= 0 {
		t.Fatalf("want a positive length, got %d", n)
	}
	if b.gotPlug != "gmail" || b.gotAcct != "acct_1" {
		t.Fatalf("broker saw plugin=%q account=%q; the plugin id must come from the host", b.gotPlug, b.gotAcct)
	}
	var out map[string]string
	if err := json.Unmarshal(host.LastResponse, &out); err != nil || out["access_token"] != "ya29.secret" {
		t.Fatalf("bad answer %s (%v)", host.LastResponse, err)
	}
	if _, has := out["refresh_token"]; has {
		t.Fatal("a refresh token must never be returned")
	}
	host.AppendLog("token is ya29.secret here")
	if strings.Contains(strings.Join(host.GetLogs(), "\n"), "ya29.secret") {
		t.Fatal("the access token must be redacted from plugin logs")
	}
}

func TestOAuthGetTokenRefusals(t *testing.T) {
	grant := TokenGrant{Provider: "google", AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour)}
	cases := []struct {
		name   string
		perms  PluginPermissions
		broker TokenBroker
		want   int32
	}{
		{"manifest lacks permission", PluginPermissions{}, &fakeBroker{grant: grant}, OAuthErrDenied},
		{"manifest lists another provider", PluginPermissions{OAuth: []string{"microsoft"}}, &fakeBroker{grant: grant}, OAuthErrDenied},
		{"account of another plugin", PluginPermissions{OAuth: []string{"google"}}, &fakeBroker{err: ErrTokenForbidden}, OAuthErrDenied},
		{"login expired", PluginPermissions{OAuth: []string{"google"}}, &fakeBroker{err: ErrTokenAuthExpired}, OAuthErrAuthExpired},
		{"broker failure", PluginPermissions{OAuth: []string{"google"}}, &fakeBroker{err: errors.New("boom")}, OAuthErrUnavailable},
		{"no broker", PluginPermissions{OAuth: []string{"google"}}, nil, OAuthErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, host, get := oauthHost(t, tc.perms, tc.broker)
			if got := get("acct_1"); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
			if len(host.LastResponse) != 0 {
				t.Fatal("nothing may be exposed on refusal")
			}
		})
	}
}

func TestOAuthGetTokenRejectsAbsurdAccountIDs(t *testing.T) {
	_, _, get := oauthHost(t, PluginPermissions{OAuth: []string{"google"}}, &fakeBroker{})
	if got := get(strings.Repeat("a", 300)); got != OAuthErrDenied {
		t.Fatalf("got %d", got)
	}
	if got := get(""); got != OAuthErrDenied {
		t.Fatalf("empty id: got %d", got)
	}
}
