//go:build wasip1

// Package wasmguest is the WebAssembly side of the provider kit: memory hand-off for the
// ABI exports, and netio implementations backed by the host's acton_net, acton_sys and
// acton_oauth imports. The //go:wasmexport functions themselves must live in the plugin's
// main package; see internal/mailcal/refplugin/guest for the pattern.
package wasmguest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"unsafe"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
)

//go:wasmimport acton_net http_request
func hostHTTPRequest(ptr, length uint32) int32

//go:wasmimport acton_sys read_response
func hostReadResponse(dest, max uint32) int32

//go:wasmimport acton_oauth get_token
func hostGetToken(ptr, length uint32) int32

var live = map[uint32][]byte{}

// Alloc is the body of the plugin's acton_alloc export: the host calls it to place a request
// in guest memory. The buffer stays referenced until the next call to Serve.
func Alloc(size uint32) uint32 {
	if size == 0 {
		size = 1
	}
	buf := make([]byte, size)
	p := ptr(buf)
	live[p] = buf
	return p
}

// Free is the body of acton_free.
func Free(p uint32) { delete(live, p) }

func ptr(b []byte) uint32 { return uint32(uintptr(unsafe.Pointer(&b[0]))) }

// Serve runs one export: it copies the request, releases buffers from earlier calls so a
// long-lived instance does not grow, dispatches, and returns the packed (ptr<<32|len) answer.
func Serve(b provkit.Backend, op string, p, length uint32) uint64 {
	req := append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(uintptr(p))), length)...)
	clear(live)
	out := provkit.Dispatch(context.Background(), b, op, req)
	q := Alloc(uint32(len(out)))
	copy(live[q], out)
	return uint64(q)<<32 | uint64(len(out))
}

// readHostResponse copies the host's pending answer (n bytes) into a fresh slice.
func readHostResponse(n int32) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	got := hostReadResponse(ptr(buf), uint32(n))
	if got < 0 || got != n {
		return nil, fmt.Errorf("short read from the host: %d of %d bytes", got, n)
	}
	return buf, nil
}

// HTTP is a netio.Doer backed by the host. The host enforces permissions.net_outbound.
type HTTP struct{}

var _ netio.Doer = HTTP{}

type httpReq struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"`
	BodyBase64 string            `json:"body_base64,omitempty"`
	Timeout    int               `json:"timeout_seconds,omitempty"`
}

type httpResp struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Error   string            `json:"error"`
}

// Do implements netio.Doer.
func (HTTP) Do(ctx context.Context, r netio.Request) (netio.Response, error) {
	if err := ctx.Err(); err != nil {
		return netio.Response{}, err
	}
	payload := httpReq{Method: r.Method, URL: r.URL, Headers: r.Headers, Timeout: r.TimeoutSeconds}
	if len(r.Body) > 0 {
		payload.BodyBase64 = base64.StdEncoding.EncodeToString(r.Body)
	}
	raw, _ := json.Marshal(payload)
	n := hostHTTPRequest(ptr(raw), uint32(len(raw)))
	if n < 0 {
		return netio.Response{}, errors.New("the host refused the request")
	}
	body, err := readHostResponse(n)
	if err != nil {
		return netio.Response{}, err
	}
	var resp httpResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return netio.Response{}, fmt.Errorf("the host sent an unreadable answer: %w", err)
	}
	if resp.Error != "" {
		// Only the host fills this field: the provider's API never saw the request.
		if resp.Status == 403 {
			return netio.Response{}, abi.Errorf(abi.ErrInvalid, "blocked by the plugin's network permissions: %s", resp.Error)
		}
		return netio.Response{}, abi.Errorf(abi.ErrTransient, "the host could not complete the request: %s", resp.Error)
	}
	return netio.Response{Status: resp.Status, Headers: resp.Headers, Body: []byte(resp.Body)}, nil
}

// Tokens is a netio.Tokens backed by acton_oauth.get_token.
type Tokens struct{}

var _ netio.Tokens = Tokens{}

// AccessToken implements netio.Tokens, mapping the host's refusal codes to coded errors.
func (Tokens) AccessToken(ctx context.Context, accountID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	id := []byte(accountID)
	if len(id) == 0 {
		return "", abi.Errorf(abi.ErrInvalid, "no account id")
	}
	n := hostGetToken(ptr(id), uint32(len(id)))
	switch {
	case n == -3:
		return "", abi.Errorf(abi.ErrAuthExpired, "sign in again")
	case n == -2:
		return "", abi.Errorf(abi.ErrInvalid, "this plugin may not use that account")
	case n < 0:
		return "", abi.Errorf(abi.ErrTransient, "no access token is available right now")
	}
	body, err := readHostResponse(n)
	if err != nil {
		return "", abi.Errorf(abi.ErrTransient, "%v", err)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", abi.Errorf(abi.ErrTransient, "the host returned no token")
	}
	return out.AccessToken, nil
}
