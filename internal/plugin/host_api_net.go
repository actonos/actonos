package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/actonos/actonos/internal/security"
	"github.com/tetratelabs/wazero/api"
)

const maxPluginHTTPBody = 32 << 20

func sandboxedHTTPClient(h *HostContext, timeout time.Duration) *http.Client {
	gate := (*SecurityGate)(nil)
	if h != nil {
		gate = h.Gate
		if h.HTTPClient != nil {
			cloned := *h.HTTPClient
			if cloned.Timeout == 0 {
				cloned.Timeout = timeout
			}
			if cloned.CheckRedirect == nil {
				cloned.CheckRedirect = pluginRedirectCheck(gate)
			}
			return &cloned
		}
	}
	var policy security.DialPolicy
	if h != nil {
		policy = h.Dial
	}
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: pluginRedirectCheck(gate),
		Transport:     rebindingSafeTransport(policy),
	}
}

// rebindingSafeTransport dials the single address the safe resolver vetted, never the name.
// CheckOutboundURLContext already looked the name up once to enforce the allowlist; without this
// the stock transport would look it up a second time and an attacker-controlled DNS server could
// answer differently (a private address) the second time.
func rebindingSafeTransport(p security.DialPolicy) *http.Transport {
	return &http.Transport{
		Proxy: nil, // a proxy would resolve the name itself, outside this policy
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(port)
			if err != nil {
				return nil, err
			}
			return p.DialTCP(ctx, host, n)
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

func pluginRedirectCheck(gate *SecurityGate) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		if gate == nil {
			return fmt.Errorf("%w: missing security gate", ErrDomainNotWhitelisted)
		}
		return gate.CheckOutboundURLContext(req.Context(), req.URL.String())
	}
}

func netHTTPRequest(ctx context.Context, m api.Module, reqPtr, reqLen uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return -1
	}

	reqBytes, err := readBufferFromMemory(m.Memory(), reqPtr, reqLen)
	if err != nil {
		h.Record("ERROR", fmt.Sprintf("http_request: read memory failed: %v", err))
		return -1
	}

	var reqPayload HTTPRequestPayload
	if err := json.Unmarshal(reqBytes, &reqPayload); err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("invalid json payload: %v", err)})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 400, Error: fmt.Sprintf("invalid json payload: %v", err), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}

	if err := h.Gate.CheckOutboundURLContext(ctx, reqPayload.URL); err != nil {
		h.Record("WARN", fmt.Sprintf("http_request blocked by gate: %s (%v)", reqPayload.URL, err))
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 403, Error: err.Error(), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}

	reqTimeout := httpRequestTimeout(reqPayload)
	client := sandboxedHTTPClient(h, reqTimeout)

	reqCtx, reqCancel := context.WithTimeout(context.Background(), reqTimeout)
	defer reqCancel()

	method := reqPayload.Method
	if method == "" {
		method = http.MethodGet
	}

	bodyBytes, err := decodeHTTPRequestBody(reqPayload)
	if err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 400, Error: err.Error(), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(reqCtx, method, reqPayload.URL, bodyReader)
	if err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 500, Error: err.Error(), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}

	for k, v := range reqPayload.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		h.Record("WARN", fmt.Sprintf("HTTP %s %s failed: %v", method, reqPayload.URL, err))
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 502, Error: err.Error(), Body: string(errBody)})
		h.mu.Lock()
		h.LastResponse = resBytes
		h.mu.Unlock()
		return int32(len(resBytes))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxPluginHTTPBody))
	h.Record("DEBUG", fmt.Sprintf("HTTP %s %s -> %d (%d bytes)", method, reqPayload.URL, resp.StatusCode, len(respBody)))
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resPayload := HTTPResponsePayload{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    string(respBody),
	}
	resBytes, _ := json.Marshal(resPayload)

	h.mu.Lock()
	h.LastResponse = resBytes
	h.mu.Unlock()

	return int32(len(resBytes))
}

func hostHTTPRequest(ctx context.Context, m api.Module, reqPtr, reqLen uint32) uint64 {
	h := HostContextFrom(ctx)
	if h == nil {
		return 0
	}

	reqBytes, err := readBufferFromMemory(m.Memory(), reqPtr, reqLen)
	if err != nil {
		h.Record("ERROR", fmt.Sprintf("host_http_request: read memory failed: %v", err))
		return 0
	}

	var reqPayload HTTPRequestPayload
	if err := json.Unmarshal(reqBytes, &reqPayload); err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("invalid json payload: %v", err)})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 400, Error: fmt.Sprintf("invalid json payload: %v", err), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}

	if err := h.Gate.CheckOutboundURLContext(ctx, reqPayload.URL); err != nil {
		h.Record("WARN", fmt.Sprintf("host_http_request blocked by gate: %s (%v)", reqPayload.URL, err))
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 403, Error: err.Error(), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}

	reqTimeout := httpRequestTimeout(reqPayload)
	client := sandboxedHTTPClient(h, reqTimeout)

	reqCtx, reqCancel := context.WithTimeout(context.Background(), reqTimeout)
	defer reqCancel()

	method := reqPayload.Method
	if method == "" {
		method = http.MethodGet
	}

	bodyBytes, err := decodeHTTPRequestBody(reqPayload)
	if err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 400, Error: err.Error(), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(reqCtx, method, reqPayload.URL, bodyReader)
	if err != nil {
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 500, Error: err.Error(), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}

	for k, v := range reqPayload.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		h.Record("WARN", fmt.Sprintf("host_http_request %s %s failed: %v", method, reqPayload.URL, err))
		errBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		resBytes, _ := json.Marshal(HTTPResponsePayload{Status: 502, Error: err.Error(), Body: string(errBody)})
		res, _ := writeBufferToGuest(ctx, h, resBytes)
		return res
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxPluginHTTPBody))
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resPayload := HTTPResponsePayload{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    string(respBody),
	}
	resBytes, _ := json.Marshal(resPayload)
	packed, _ := writeBufferToGuest(ctx, h, resBytes)
	return packed
}
