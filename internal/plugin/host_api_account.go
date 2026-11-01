package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// Errors a CredentialBroker returns so the host can pick the right code.
var ErrCredentialsForbidden = errors.New("the account does not belong to this plugin")

// Credentials are the login of a password account. They exist only inside the broker, the host
// call and the guest's memory for the duration of one operation.
type Credentials struct {
	Username string
	Password string
}

// CredentialBroker hands out the credentials of an account. Implementations must verify that
// accountID belongs to pluginID: the plugin id comes from the host context, never from the guest.
type CredentialBroker interface {
	Credentials(ctx context.Context, pluginID, accountID string) (Credentials, error)
}

// SetCredentialBroker sets the broker used by plugins loaded afterwards.
func (m *Manager) SetCredentialBroker(b CredentialBroker) {
	m.mu.Lock()
	m.credentials = b
	m.mu.Unlock()
}

// Return codes of acton_account.get_credentials (a positive value is the answer's length).
const (
	CredErrNoHost      int32 = -1
	CredErrDenied      int32 = -2
	CredErrUnavailable int32 = -4
)

// accountGetCredentials implements acton_account.get_credentials(accountPtr, accountLen) -> i32.
func accountGetCredentials(ctx context.Context, m api.Module, ptr, length uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil || h.Gate == nil {
		return CredErrNoHost
	}
	if err := h.Gate.CheckCredentialsAccess(); err != nil {
		h.Record("WARN", err.Error())
		return CredErrDenied
	}
	if h.Credentials == nil {
		return CredErrUnavailable
	}
	raw, err := readBufferFromMemory(m.Memory(), ptr, length)
	if err != nil || len(raw) == 0 || len(raw) > 256 {
		return CredErrDenied
	}
	creds, err := h.Credentials.Credentials(ctx, h.PluginID, string(raw))
	switch {
	case errors.Is(err, ErrCredentialsForbidden):
		h.Record("WARN", "credentials request refused: account does not belong to this plugin")
		return CredErrDenied
	case err != nil:
		h.Record("WARN", fmt.Sprintf("credentials unavailable: %v", err))
		return CredErrUnavailable
	}
	h.rememberSecret(creds.Password) // never appears in plugin logs
	body, _ := json.Marshal(map[string]string{"username": creds.Username, "password": creds.Password})
	h.mu.Lock()
	h.LastResponse = body
	h.mu.Unlock()
	return int32(len(body))
}
