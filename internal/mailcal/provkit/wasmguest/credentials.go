//go:build wasip1

package wasmguest

import (
	"context"
	"encoding/json"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
)

//go:wasmimport acton_account get_credentials
func hostGetCredentials(ptr, length uint32) int32

// Credentials is a netio.Credentials backed by acton_account.get_credentials.
type Credentials struct{}

var _ netio.Credentials = Credentials{}

// Credentials implements netio.Credentials. The caller should keep the login only as long as
// one operation needs it.
func (Credentials) Credentials(ctx context.Context, accountID string) (netio.Login, error) {
	if err := ctx.Err(); err != nil {
		return netio.Login{}, err
	}
	id := []byte(accountID)
	if len(id) == 0 {
		return netio.Login{}, abi.Errorf(abi.ErrInvalid, "no account id")
	}
	n := hostGetCredentials(ptr(id), uint32(len(id)))
	switch {
	case n == -2:
		return netio.Login{}, abi.Errorf(abi.ErrInvalid, "this plugin may not use the credentials of that account")
	case n < 0:
		return netio.Login{}, abi.Errorf(abi.ErrAuthExpired, "the account has no usable password")
	}
	body, err := readHostResponse(n)
	if err != nil {
		return netio.Login{}, abi.Errorf(abi.ErrTransient, "%v", err)
	}
	var out struct{ Username, Password string }
	if err := json.Unmarshal(body, &out); err != nil || out.Password == "" {
		return netio.Login{}, abi.Errorf(abi.ErrAuthExpired, "the account has no usable password")
	}
	return netio.Login{Username: out.Username, Password: out.Password}, nil
}
