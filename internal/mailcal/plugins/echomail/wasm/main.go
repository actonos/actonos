//go:build wasip1

// Command wasm is the example echo-mail provider plugin:
//
//	GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildmode=c-shared -o plugin.wasm ./internal/mailcal/plugins/echomail/wasm
package main

import (
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/plugins/echomail"
	"github.com/actonos/actonos/internal/mailcal/provkit/wasmguest"
)

func main() {}

var backend = echomail.Backend{}

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

//go:wasmexport acton_mail_search
func mailSearch(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailSearch, p, l) }

//go:wasmexport acton_mail_get
func mailGet(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailGet, p, l) }

//go:wasmexport acton_mail_get_thread
func mailGetThread(p, l uint32) uint64 {
	return wasmguest.Serve(backend, abi.ExportMailGetThread, p, l)
}
