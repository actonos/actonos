package mailcal_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/auth"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/refstore"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/plugin"
)

type staticClients struct{ id, secret string }

func (c staticClients) Client(context.Context, string) (string, string, error) {
	return c.id, c.secret, nil
}

type oauthEnv struct {
	svc     *mailcal.Service
	flow    *mailcal.OAuthFlow
	daemon  *auth.TokenRefreshDaemon
	broker  *mailcal.TokenBroker
	account mailcal.Account
	token   *atomic.Value // what the fake token endpoint returns next
	hits    atomic.Int32
}

func newOAuthEnv(t *testing.T, refreshToken string) *oauthEnv {
	t.Helper()
	env := &oauthEnv{token: &atomic.Value{}}
	env.token.Store(refreshToken)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env.hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at-1", "refresh_token": env.token.Load().(string), "token_type": "Bearer", "expires_in": 3600})
	}))
	t.Cleanup(srv.Close)

	db, err := memory.Open(filepath.Join(t.TempDir(), "oauth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	vault, err := memory.NewVault(db, "vault-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	eb := bus.NewEventBus()
	t.Cleanup(eb.Close)
	states := auth.NewStateStore(time.Minute)
	engine := auth.NewOAuthEngineWithClient(states, srv.Client())
	env.daemon = auth.NewTokenRefreshDaemon(engine, vault, db, eb)

	env.svc, err = mailcal.NewService(db.SQLDB())
	if err != nil {
		t.Fatal(err)
	}
	manifest := plugin.PluginManifest{ID: "gmail", Name: "Gmail", Version: "1", Capabilities: []string{"mail@1", "calendar@1"},
		OAuth: &plugin.PluginOAuth{Provider: "google", AuthURL: "https://accounts.example/auth", TokenURL: srv.URL + "/token", Scopes: []string{"mail"}}}
	env.svc.PluginActivated(context.Background(), manifest, mailcal.StoreCaller{Store: refstore.New(refstore.DefaultSeed())})
	env.account, err = env.svc.Accounts().Create(context.Background(), mailcal.NewAccount{
		Label: "G", PluginID: "gmail", Kind: mailcal.KindBoth, Email: "me@example.com", Auth: mailcal.AuthRef{Type: "oauth"}})
	if err != nil {
		t.Fatal(err)
	}
	env.flow = mailcal.NewOAuthFlow(env.svc, engine, states, env.daemon, staticClients{"cid", "csecret"})
	env.flow.SetURLCheckForTest(func(context.Context, string) error { return nil })
	env.broker = mailcal.NewTokenBroker(env.svc.Accounts(), env.daemon, env.svc.OAuthSpec)
	return env
}

func stateOf(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil || u.Query().Get("state") == "" {
		t.Fatalf("bad auth url %q", authURL)
	}
	if u.Query().Get("code_challenge_method") != "S256" {
		t.Fatal("PKCE S256 is required")
	}
	return u.Query().Get("state")
}

func TestSignInStoresTheTokenUnderThePerAccountKey(t *testing.T) {
	env := newOAuthEnv(t, "refresh-1")
	ctx := context.Background()
	authURL, nonce, err := env.flow.Start(ctx, env.account.ID, "https://acton.local/api/mailcal/oauth/callback")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := env.svc.Accounts().Get(ctx, env.account.ID); got.Status != mailcal.StatusAuthExpired {
		t.Fatalf("an account waiting for sign-in is not usable yet: %s", got.Status)
	}
	id, err := env.flow.Complete(ctx, stateOf(t, authURL), "the-code", nonce)
	if err != nil || id != env.account.ID {
		t.Fatalf("complete: %q %v", id, err)
	}
	tok, err := env.daemon.GetToken(ctx, mailcal.ProviderKey(env.account.ID))
	if err != nil || tok.RefreshToken != "refresh-1" {
		t.Fatalf("token not stored under the account key: %+v %v", tok, err)
	}
	if got, _ := env.svc.Accounts().Get(ctx, env.account.ID); got.Status != mailcal.StatusOK {
		t.Fatalf("status = %s", got.Status)
	}
	grant, err := env.broker.AccessToken(ctx, "gmail", env.account.ID)
	if err != nil || grant.AccessToken != "at-1" || grant.Provider != "google" {
		t.Fatalf("grant = %+v %v", grant, err)
	}
}

func TestSignInRejectsAForeignBrowserAndCannotBeReplayed(t *testing.T) {
	env := newOAuthEnv(t, "refresh-1")
	ctx := context.Background()
	authURL, nonce, _ := env.flow.Start(ctx, env.account.ID, "https://acton.local/cb")
	state := stateOf(t, authURL)

	// A victim's browser arrives with the attacker's state but without the attacker's nonce cookie.
	if _, err := env.flow.Complete(ctx, state, "attacker-code", "not-the-nonce"); !errors.Is(err, mailcal.ErrOAuthState) {
		t.Fatalf("a callback without the right nonce must fail: %v", err)
	}
	// The state was consumed by that attempt, so even the right nonce cannot be used afterwards.
	if _, err := env.flow.Complete(ctx, state, "code", nonce); !errors.Is(err, mailcal.ErrOAuthState) {
		t.Fatalf("a state is single-use: %v", err)
	}
	if env.hits.Load() != 0 {
		t.Fatal("no token request may be made for a rejected callback")
	}
	if _, err := env.flow.Complete(ctx, "made-up-state", "code", nonce); !errors.Is(err, mailcal.ErrOAuthState) {
		t.Fatalf("unknown state: %v", err)
	}
}

func TestSignInNeedsARefreshToken(t *testing.T) {
	env := newOAuthEnv(t, "")
	ctx := context.Background()
	authURL, nonce, _ := env.flow.Start(ctx, env.account.ID, "https://acton.local/cb")
	if _, err := env.flow.Complete(ctx, stateOf(t, authURL), "code", nonce); !errors.Is(err, mailcal.ErrNoRefreshToken) {
		t.Fatalf("got %v", err)
	}
	if got, _ := env.svc.Accounts().Get(ctx, env.account.ID); got.Status == mailcal.StatusOK {
		t.Fatal("the account must not look connected")
	}
}

func TestTokenEndpointsMustBeHTTPSAndPublic(t *testing.T) {
	env := newOAuthEnv(t, "r")
	ctx := context.Background()
	strict := mailcal.NewOAuthFlow(env.svc, auth.NewOAuthEngine(auth.NewStateStore(time.Minute)), auth.NewStateStore(time.Minute), env.daemon, staticClients{"cid", "s"})
	// The default check refuses a loopback token_url: the test server lives on 127.0.0.1.
	if _, _, err := strict.Start(ctx, env.account.ID, "https://acton.local/cb"); err == nil {
		t.Fatal("a token endpoint on loopback must be refused")
	}
}

func TestBrokerNeverServesAnotherPluginsAccount(t *testing.T) {
	env := newOAuthEnv(t, "r")
	ctx := context.Background()
	authURL, nonce, _ := env.flow.Start(ctx, env.account.ID, "https://acton.local/cb")
	if _, err := env.flow.Complete(ctx, stateOf(t, authURL), "c", nonce); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ plugin, account string }{{"evilplugin", env.account.ID}, {"gmail", "acct_does_not_exist"}} {
		if _, err := env.broker.AccessToken(ctx, tc.plugin, tc.account); !errors.Is(err, plugin.ErrTokenForbidden) {
			t.Errorf("plugin=%s account=%s: want ErrTokenForbidden, got %v", tc.plugin, tc.account, err)
		}
	}
}

type fakeSource struct {
	tok       *auth.StoredTokenData
	getErr    error
	refreshE  error
	refreshes int
}

func (f *fakeSource) GetToken(context.Context, string) (*auth.StoredTokenData, error) {
	return f.tok, f.getErr
}
func (f *fakeSource) RefreshNow(context.Context, string) (*auth.StoredTokenData, error) {
	f.refreshes++
	if f.refreshE != nil {
		return nil, f.refreshE
	}
	f.tok = &auth.StoredTokenData{AccessToken: "renewed", ExpiresAt: time.Now().Add(time.Hour)}
	return f.tok, nil
}

func TestBrokerRenewsNearExpiryAndMarksRevokedLogins(t *testing.T) {
	env := newOAuthEnv(t, "r")
	ctx := context.Background()
	_ = env.svc.Accounts().SetStatus(ctx, env.account.ID, mailcal.StatusOK, "")

	src := &fakeSource{tok: &auth.StoredTokenData{AccessToken: "old", ExpiresAt: time.Now().Add(10 * time.Second)}}
	b := mailcal.NewTokenBroker(env.svc.Accounts(), src, env.svc.OAuthSpec)
	grant, err := b.AccessToken(ctx, "gmail", env.account.ID)
	if err != nil || grant.AccessToken != "renewed" || src.refreshes != 1 {
		t.Fatalf("grant=%+v err=%v refreshes=%d", grant, err, src.refreshes)
	}

	src = &fakeSource{tok: &auth.StoredTokenData{AccessToken: "old", ExpiresAt: time.Now().Add(time.Second)}, refreshE: auth.ErrTokenRefresh}
	b = mailcal.NewTokenBroker(env.svc.Accounts(), src, env.svc.OAuthSpec)
	if _, err := b.AccessToken(ctx, "gmail", env.account.ID); !errors.Is(err, plugin.ErrTokenAuthExpired) {
		t.Fatalf("got %v", err)
	}
	if got, _ := env.svc.Accounts().Get(ctx, env.account.ID); got.Status != mailcal.StatusAuthExpired {
		t.Fatalf("a revoked login must mark the account: %s", got.Status)
	}
	// And further requests do not hammer the provider.
	src.refreshes = 0
	if _, err := b.AccessToken(ctx, "gmail", env.account.ID); !errors.Is(err, plugin.ErrTokenAuthExpired) || src.refreshes != 0 {
		t.Fatalf("expired accounts must short-circuit: err=%v refreshes=%d", err, src.refreshes)
	}
}

func TestDeletingAnAccountDeletesItsToken(t *testing.T) {
	env := newOAuthEnv(t, "refresh-1")
	ctx := context.Background()
	authURL, nonce, _ := env.flow.Start(ctx, env.account.ID, "https://acton.local/cb")
	if _, err := env.flow.Complete(ctx, stateOf(t, authURL), "c", nonce); err != nil {
		t.Fatal(err)
	}
	if err := env.flow.Forget(ctx, env.account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.daemon.GetToken(ctx, mailcal.ProviderKey(env.account.ID)); err == nil {
		t.Fatal("the token must be gone with its account")
	}
}

func TestProvidersAdvertiseTheirSignInProvider(t *testing.T) {
	env := newOAuthEnv(t, "r")
	list := env.svc.Providers()
	if len(list) != 1 || list[0].OAuthProvider != "google" || list[0].AuthKind != "oauth" {
		t.Fatalf("%+v", list)
	}
}
