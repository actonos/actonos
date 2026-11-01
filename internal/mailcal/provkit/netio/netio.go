// Package netio is what provider plugins use to reach the network and to get an access token.
// In a WebAssembly build it is backed by the host (package wasmguest); in tests and native
// builds it is backed by net/http. A provider written against these interfaces is testable
// without WebAssembly.
package netio

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

// Request is one HTTP request.
type Request struct {
	Method         string
	URL            string
	Headers        map[string]string
	Body           []byte
	TimeoutSeconds int
}

// Response is one HTTP answer. A non-2xx status is NOT an error here; use StatusError.
type Response struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

// Doer performs requests.
type Doer interface {
	Do(ctx context.Context, req Request) (Response, error)
}

// Tokens returns a short-lived access token for one of the plugin's accounts.
type Tokens interface {
	AccessToken(ctx context.Context, accountID string) (string, error)
}

// Header looks a response header up case-insensitively.
func (r Response) Header(name string) string {
	for k, v := range r.Headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// StatusError maps a failed HTTP answer to the ABI's coded errors, or returns nil for 2xx.
// Providers call it on every response so the core can retry, pause or report correctly.
func StatusError(r Response, what string) *abi.Error {
	if r.Status >= 200 && r.Status < 300 {
		return nil
	}
	detail := snippet(r.Body)
	switch {
	case r.Status == 401:
		return abi.Errorf(abi.ErrAuthExpired, "%s: the provider rejected the access token", what)
	case r.Status == 404 || r.Status == 410:
		return abi.Errorf(abi.ErrNotFound, "%s: not found", what)
	case r.Status == 429 || (r.Status == 403 && strings.Contains(strings.ToLower(detail), "ratelimit")):
		e := abi.Errorf(abi.ErrRateLimited, "%s: rate limited", what)
		if secs, err := strconv.Atoi(strings.TrimSpace(r.Header("Retry-After"))); err == nil && secs >= 0 {
			e.RetryAfterMS = int64(secs) * 1000
		}
		return e
	case r.Status == 403:
		return abi.Errorf(abi.ErrInvalid, "%s: the account is not allowed to do this (%s)", what, detail)
	case r.Status == 408 || r.Status >= 500:
		return abi.Errorf(abi.ErrTransient, "%s: the provider failed (HTTP %d)", what, r.Status)
	}
	return abi.Errorf(abi.ErrInvalid, "%s: HTTP %d: %s", what, r.Status, detail)
}

// snippet keeps an error body short and printable so it can sit in a log or a tool result.
func snippet(b []byte) string {
	s := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, string(b))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return strings.TrimSpace(s)
}

// Describe formats a request for errors (never includes headers, which carry the token).
func Describe(r Request) string { return fmt.Sprintf("%s %s", r.Method, r.URL) }
