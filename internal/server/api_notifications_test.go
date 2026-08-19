package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPushNotificationsAPI(t *testing.T) {
	srv := newTestServer(t)

	// 1. Get VAPID public key
	req := httptest.NewRequest(http.MethodGet, "/api/notifications/push/vapid-key", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for vapid-key, got %d: %s", w.Code, w.Body.String())
	}

	var vapidRes struct {
		Data struct {
			PublicKey string `json:"public_key"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&vapidRes); err != nil {
		t.Fatalf("failed to decode vapid key response: %v", err)
	}
	if vapidRes.Data.PublicKey == "" {
		t.Fatal("expected non-empty VAPID public key from API")
	}

	// 2. Subscribe to push
	subPayload := map[string]any{
		"endpoint": "https://updates.push.services.mozilla.com/wpush/v2/test-endpoint-abc",
		"keys": map[string]string{
			"p256dh": "BN3-sample-p256dh-key",
			"auth":   "auth-secret-123",
		},
		"user_agent": "Mozilla/5.0 TestBrowser",
	}
	subBody, _ := json.Marshal(subPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/notifications/push/subscribe", bytes.NewReader(subBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for subscribe, got %d: %s", w.Code, w.Body.String())
	}

	// 3. Trigger test push notification
	testPayload := map[string]any{
		"title":   "Test Alert",
		"message": "Testing Service Worker push",
		"link":    "/notifications",
	}
	testBody, _ := json.Marshal(testPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/notifications/push/test", bytes.NewReader(testBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for push test, got %d: %s", w.Code, w.Body.String())
	}

	// 4. Unsubscribe
	unsubPayload := map[string]any{
		"endpoint": "https://updates.push.services.mozilla.com/wpush/v2/test-endpoint-abc",
	}
	unsubBody, _ := json.Marshal(unsubPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/notifications/push/unsubscribe", bytes.NewReader(unsubBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for unsubscribe, got %d: %s", w.Code, w.Body.String())
	}
}
