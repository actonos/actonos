//go:build wasip1

// Command netguest is a probe plugin used only by tests: it proves that a WebAssembly provider
// can obtain an access token and call the network through the host, and nothing more.
package main

import (
	"context"
	"fmt"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/provkit/netio"
	"github.com/actonos/actonos/internal/mailcal/provkit/wasmguest"
)

func main() {}

type probe struct {
	provkit.Base
	http   netio.Doer
	tokens netio.Tokens
	creds  netio.Credentials
}

var backend = &probe{http: wasmguest.HTTP{}, tokens: wasmguest.Tokens{}, creds: wasmguest.Credentials{}}

func (p *probe) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{ABI: []string{abi.CapMail}, Ops: []string{abi.ExportCapabilities, abi.ExportMailList}}, nil
}

// MailList asks for a token, calls account.config.url with it, and reports what came back as
// the subject of one message.
func (p *probe) MailList(ctx context.Context, acct abi.AccountRef, _ abi.MailListParams) (abi.MailListResult, error) {
	tok, err := p.tokens.AccessToken(ctx, acct.ID)
	if err != nil {
		return abi.MailListResult{}, err
	}
	url, _ := acct.Config["url"].(string)
	resp, err := p.http.Do(ctx, netio.Request{Method: "GET", URL: url, Headers: map[string]string{"Authorization": "Bearer " + tok}})
	if err != nil {
		return abi.MailListResult{}, err
	}
	if e := netio.StatusError(resp, "probe"); e != nil {
		return abi.MailListResult{}, e
	}
	return abi.MailListResult{Messages: []abi.Message{{ID: "p1", Subject: fmt.Sprintf("%d %s", resp.Status, resp.Body)}}}, nil
}

// MailGet reports the username and the LENGTH of the password, never the password itself.
func (p *probe) MailGet(ctx context.Context, acct abi.AccountRef, _ string) (abi.Message, error) {
	login, err := p.creds.Credentials(ctx, acct.ID)
	if err != nil {
		return abi.Message{}, err
	}
	return abi.Message{ID: "c1", Subject: login.Username, BodyText: fmt.Sprintf("password length %d", len(login.Password))}, nil
}

//go:wasmexport acton_alloc
func actonAlloc(size uint32) uint32 { return wasmguest.Alloc(size) }

//go:wasmexport acton_free
func actonFree(ptr uint32) { wasmguest.Free(ptr) }

//go:wasmexport acton_plugin_init
func actonPluginInit(ptr, length uint32) uint32 { return 0 }

//go:wasmexport acton_mailcal_capabilities
func capabilities(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportCapabilities, p, l) }

//go:wasmexport acton_mail_get
func mailGet(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailGet, p, l) }

//go:wasmexport acton_mail_list
func mailList(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailList, p, l) }
