package server

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
)

func TestMailcalPasswordGoesToTheVaultAndIsNeverEchoed(t *testing.T) {
	srv := newTestServer(t)
	srv.mailcalCreds = mailcal.NewCredentialBroker(srv.mailcal.Accounts(), srv.vault)

	status, env := call(t, srv, http.MethodPost, "/api/mailcal/accounts", `{"label":"Fastmail","plugin_id":"refmem","kind":"both","email":"me@example.com"}`)
	if status != http.StatusCreated {
		t.Fatalf("create: %d %v", status, env)
	}
	id := dataOf(t, env)["id"].(string)

	status, env = call(t, srv, http.MethodPut, "/api/mailcal/accounts/"+id+"/password", `{"password":"s3cret-app-pw"}`)
	if status != http.StatusOK || dataOf(t, env)["configured"] != true {
		t.Fatalf("put: %d %v", status, env)
	}
	for _, path := range []string{"/api/mailcal/accounts/" + id, "/api/mailcal/accounts"} {
		_, body := call(t, srv, http.MethodGet, path, "")
		if strings.Contains(fmt.Sprint(body), "s3cret-app-pw") {
			t.Fatalf("%s leaked the password: %v", path, body)
		}
	}
	got, err := srv.vault.GetSecret(t.Context(), mailcal.PasswordKey(id))
	if err != nil || got != "s3cret-app-pw" {
		t.Fatalf("vault: %q %v", got, err)
	}
	if status, _ := call(t, srv, http.MethodPut, "/api/mailcal/accounts/"+id+"/password", `{"password":""}`); status != http.StatusBadRequest {
		t.Fatalf("an empty password is a 400, got %d", status)
	}
	call(t, srv, http.MethodDelete, "/api/mailcal/accounts/"+id, "")
	if _, err := srv.vault.GetSecret(t.Context(), mailcal.PasswordKey(id)); err == nil {
		t.Fatal("deleting the account must delete its password")
	}
}
