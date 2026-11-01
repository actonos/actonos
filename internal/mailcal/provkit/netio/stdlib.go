//go:build !wasip1

package netio

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

// Stdlib adapts net/http to Doer for tests and native builds.
type Stdlib struct{ Client *http.Client }

// Do implements Doer.
func (s Stdlib) Do(ctx context.Context, r Request) (Response, error) {
	c := s.Client
	if c == nil {
		c = &http.Client{Timeout: 60 * time.Second}
	}
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if len(r.Body) > 0 {
		body = bytes.NewReader(r.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.URL, body)
	if err != nil {
		return Response{}, err
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return Response{}, err
	}
	out := Response{Status: resp.StatusCode, Body: raw, Headers: map[string]string{}}
	for k, v := range resp.Header {
		if len(v) > 0 {
			out.Headers[k] = v[0]
		}
	}
	return out, nil
}

// StaticToken returns the same token for every account (tests).
type StaticToken string

// AccessToken implements Tokens.
func (t StaticToken) AccessToken(context.Context, string) (string, error) { return string(t), nil }
