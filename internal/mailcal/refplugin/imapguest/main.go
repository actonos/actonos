//go:build wasip1

// Command imapguest is a probe plugin used only by tests: it proves that a stock IMAP client
// library works inside WebAssembly on top of the host's TLS socket calls.
package main

import (
	"context"
	"fmt"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/provkit"
	"github.com/actonos/actonos/internal/mailcal/provkit/wasmguest"
)

func main() {}

type probe struct{ provkit.Base }

var backend = &probe{}

func (p *probe) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{ABI: []string{abi.CapMail}, Ops: []string{abi.ExportCapabilities, abi.ExportMailList}}, nil
}

func (p *probe) MailList(_ context.Context, acct abi.AccountRef, _ abi.MailListParams) (abi.MailListResult, error) {
	host, _ := acct.Config["host"].(string)
	port, _ := acct.Config["port"].(float64)
	conn, err := wasmguest.DialTCP(host, int(port), wasmguest.ModeIMAPStartTLS)
	if err != nil {
		return abi.MailListResult{}, abi.Errorf(abi.ErrTransient, "%v", err)
	}
	c := imapclient.New(conn, nil)
	defer c.Close()
	if err := c.Login("user", "secret").Wait(); err != nil {
		return abi.MailListResult{}, abi.Errorf(abi.ErrAuthExpired, "login: %v", err)
	}
	sel, err := c.Select("INBOX", nil).Wait()
	if err != nil {
		return abi.MailListResult{}, abi.Errorf(abi.ErrTransient, "select: %v", err)
	}
	_ = c.Logout().Wait()
	return abi.MailListResult{Messages: []abi.Message{{ID: "i1", Subject: fmt.Sprintf("%d messages", sel.NumMessages)}}}, nil
}

//go:wasmexport acton_alloc
func actonAlloc(size uint32) uint32 { return wasmguest.Alloc(size) }

//go:wasmexport acton_free
func actonFree(ptr uint32) { wasmguest.Free(ptr) }

//go:wasmexport acton_plugin_init
func actonPluginInit(ptr, length uint32) uint32 { return 0 }

//go:wasmexport acton_mailcal_capabilities
func capabilities(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportCapabilities, p, l) }

//go:wasmexport acton_mail_list
func mailList(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailList, p, l) }
