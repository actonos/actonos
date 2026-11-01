//go:build wasip1

// Command wasm is the IMAP and SMTP provider plugin:
//
//	GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildmode=c-shared -o plugin.wasm ./internal/mailcal/plugins/imapsmtp/wasm
//
// All behaviour lives in package imapsmtp; this file only exposes the ABI exports and connects
// the provider to the host's TLS sockets.
package main

import (
	"context"
	"net"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/plugins/imapsmtp"
	"github.com/actonos/actonos/internal/mailcal/provkit/wasmguest"
)

func main() {}

func dial(_ context.Context, host string, port int, mode string) (net.Conn, error) {
	return wasmguest.DialTCP(host, port, mode)
}

var backend = &imapsmtp.Backend{Dial: dial, Creds: wasmguest.Credentials{}}

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

//go:wasmexport acton_mail_get
func mailGet(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailGet, p, l) }

//go:wasmexport acton_mail_get_thread
func mailGetThread(p, l uint32) uint64 {
	return wasmguest.Serve(backend, abi.ExportMailGetThread, p, l)
}

//go:wasmexport acton_mail_search
func mailSearch(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailSearch, p, l) }

//go:wasmexport acton_mail_send
func mailSend(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailSend, p, l) }

//go:wasmexport acton_mail_mark
func mailMark(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailMark, p, l) }

//go:wasmexport acton_mail_move
func mailMove(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailMove, p, l) }

//go:wasmexport acton_mail_delete
func mailDelete(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailDelete, p, l) }
