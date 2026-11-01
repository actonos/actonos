//go:build wasip1

// Command wasm is the Microsoft Graph (Outlook mail and calendar) provider plugin:
//
//	GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildmode=c-shared -o plugin.wasm ./internal/mailcal/plugins/graph/wasm
//
// All behaviour lives in package graph; this file only exposes the ABI exports.
package main

import (
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/plugins/graph"
	"github.com/actonos/actonos/internal/mailcal/provkit/wasmguest"
)

func main() {}

var backend = &graph.Backend{HTTP: wasmguest.HTTP{}, Tokens: wasmguest.Tokens{}}

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

//go:wasmexport acton_mail_create_draft
func mailCreateDraft(p, l uint32) uint64 {
	return wasmguest.Serve(backend, abi.ExportMailCreateDraft, p, l)
}

//go:wasmexport acton_mail_send
func mailSend(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailSend, p, l) }

//go:wasmexport acton_mail_mark
func mailMark(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailMark, p, l) }

//go:wasmexport acton_mail_move
func mailMove(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailMove, p, l) }

//go:wasmexport acton_mail_delete
func mailDelete(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportMailDelete, p, l) }

//go:wasmexport acton_calendar_list_events
func calList(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportCalListEvents, p, l) }

//go:wasmexport acton_calendar_get_event
func calGet(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportCalGetEvent, p, l) }

//go:wasmexport acton_calendar_create_event
func calCreate(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportCalCreateEvent, p, l) }

//go:wasmexport acton_calendar_update_event
func calUpdate(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportCalUpdateEvent, p, l) }

//go:wasmexport acton_calendar_respond
func calRespond(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportCalRespond, p, l) }

//go:wasmexport acton_calendar_delete_event
func calDelete(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportCalDeleteEvent, p, l) }

//go:wasmexport acton_calendar_free_busy
func calFreeBusy(p, l uint32) uint64 { return wasmguest.Serve(backend, abi.ExportCalFreeBusy, p, l) }
