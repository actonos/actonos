package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_IntegrationsChannelsPairingAndWebhookValidation(t *testing.T) {
	srv := newTestServer(t)
	for _, endpoint := range []string{
		"/api/integrations/channels",
		"/api/integrations/channels/accounts",
		"/api/integrations/authorizations",
	} {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s failed: %d %s", endpoint, w.Code, w.Body.String())
		}
	}

	channelBody := `{
		"webhook_secret":"integration-secret",
		"telegram_accounts":[{"id":"tg_test","name":"Test Bot","channel":"telegram","token":"tg-token","enabled":true,"bound_agent_ids":["*"]}],
		"discord_accounts":[{"id":"dc_test","name":"Discord","channel":"discord","token":"dc-token","enabled":false}],
		"whatsapp_accounts":[{"id":"wa_test","name":"WhatsApp","channel":"whatsapp","token":"wa-token","phone_id":"phone","enabled":true}]
	}`
	saveChannels := httptest.NewRequest(http.MethodPost, "/api/integrations/channels", strings.NewReader(channelBody))
	saveChannels.Header.Set("Content-Type", "application/json")
	channelResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(channelResult, saveChannels)
	if channelResult.Code != http.StatusOK {
		t.Fatalf("channel save failed: %d %s", channelResult.Code, channelResult.Body.String())
	}
	accounts := httptest.NewRequest(http.MethodGet, "/api/integrations/channels/accounts", nil)
	accountsResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(accountsResult, accounts)
	if accountsResult.Code != http.StatusOK || !strings.Contains(accountsResult.Body.String(), "tg_test") ||
		strings.Contains(accountsResult.Body.String(), "tg-token") {
		t.Fatalf("channel accounts were not returned safely: %d %s", accountsResult.Code, accountsResult.Body.String())
	}

	pair := httptest.NewRequest(http.MethodPost, "/api/integrations/pairing/code", strings.NewReader(`{"channel_id":"telegram"}`))
	pair.Header.Set("Content-Type", "application/json")
	pairResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(pairResult, pair)
	if pairResult.Code != http.StatusOK {
		t.Fatalf("pairing code failed: %d %s", pairResult.Code, pairResult.Body.String())
	}
	var pairEnvelope struct {
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	_ = json.NewDecoder(pairResult.Body).Decode(&pairEnvelope)
	verifyBody := fmt.Sprintf(`{"channel_id":"telegram","code":%q,"sender_id":"user-1","sender_name":"Tester"}`, pairEnvelope.Data.Code)
	verify := httptest.NewRequest(http.MethodPost, "/api/integrations/pairing/verify", strings.NewReader(verifyBody))
	verify.Header.Set("Content-Type", "application/json")
	verifyResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(verifyResult, verify)
	if verifyResult.Code != http.StatusOK {
		t.Fatalf("pairing verification failed: %d %s", verifyResult.Code, verifyResult.Body.String())
	}
	revoke := httptest.NewRequest(http.MethodDelete, "/api/integrations/authorizations", strings.NewReader(`{"channel_id":"telegram","sender_id":"user-1"}`))
	revoke.Header.Set("Content-Type", "application/json")
	revokeResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(revokeResult, revoke)
	if revokeResult.Code != http.StatusOK {
		t.Fatalf("authorization revoke failed: %d %s", revokeResult.Code, revokeResult.Body.String())
	}
}

func TestServer_ChannelAccountsFollowStoredFilesNotHardcodedBrands(t *testing.T) {
	srv := newTestServer(t)

	save := httptest.NewRequest(http.MethodPost, "/api/integrations/channels", strings.NewReader(
		`{"zalo_accounts":[{"id":"zalo_bot","name":"Zalo","channel":"zalo","token":"zalo-token","enabled":true,"bound_agent_ids":["*"]}]}`,
	))
	save.Header.Set("Content-Type", "application/json")
	saveResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(saveResult, save)
	if saveResult.Code != http.StatusOK {
		t.Fatalf("zalo account save failed: %d %s", saveResult.Code, saveResult.Body.String())
	}

	accounts := httptest.NewRequest(http.MethodGet, "/api/integrations/channels/accounts", nil)
	accountsResult := httptest.NewRecorder()
	srv.Router().ServeHTTP(accountsResult, accounts)
	body := accountsResult.Body.String()
	if accountsResult.Code != http.StatusOK || !strings.Contains(body, "zalo_bot") || strings.Contains(body, "zalo-token") {
		t.Fatalf("zalo accounts were not listed safely: %d %s", accountsResult.Code, body)
	}
	if strings.Contains(body, `"channel":"telegram"`) && strings.Contains(body, "tg_default") {
		t.Fatalf("list invented a telegram account: %s", body)
	}

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{name: "missing", body: `{}`, want: http.StatusBadRequest},
		{name: "empty", body: `{"channel_id":""}`, want: http.StatusBadRequest},
		{name: "zalo", body: `{"channel_id":"zalo"}`, want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/integrations/pairing/code", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.Router().ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("pairing code %s: got %d %s, want %d", tc.name, w.Code, w.Body.String(), tc.want)
			}
			if tc.want == http.StatusOK && strings.Contains(w.Body.String(), `"channel_id":"telegram"`) {
				t.Fatalf("pairing code defaulted to telegram: %s", w.Body.String())
			}
		})
	}
}
