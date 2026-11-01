//go:build wasip1

// Command guest is the reference mail/calendar provider compiled to WebAssembly:
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o refplugin.wasm ./internal/mailcal/refplugin/guest
//
// It serves the deterministic in-memory dataset from package refstore through the real
// ABI, so the conformance suite exercises the host bridge, JSON framing and guest memory
// handling end to end without any network.
package main

import (
	"encoding/json"
	"unsafe"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/refstore"
)

var (
	store *refstore.Store
	live  = map[uint32][]byte{}
)

func main() {}

//go:wasmexport acton_alloc
func actonAlloc(size uint32) uint32 {
	if size == 0 {
		size = 1
	}
	buf := make([]byte, size)
	ptr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	live[ptr] = buf
	return ptr
}

//go:wasmexport acton_free
func actonFree(ptr uint32) { delete(live, ptr) }

// acton_plugin_init receives the manifest config. {"seed": {...}} replaces the default dataset.
//
//go:wasmexport acton_plugin_init
func actonPluginInit(ptr, length uint32) uint32 {
	seed := refstore.DefaultSeed()
	if length > 0 {
		var cfg struct {
			Seed *refstore.Seed `json:"seed"`
		}
		raw := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
		if err := json.Unmarshal(raw, &cfg); err == nil && cfg.Seed != nil {
			seed = *cfg.Seed
		}
	}
	store = refstore.New(seed)
	return 0
}

// serve answers one export. The request is copied first, then every buffer from earlier
// calls (including the request) is released so a long-lived instance does not grow.
func serve(op string, ptr, length uint32) uint64 {
	req := append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)...)
	clear(live)
	if store == nil {
		store = refstore.New(refstore.DefaultSeed())
	}
	out := store.Handle(op, req)
	p := actonAlloc(uint32(len(out)))
	copy(live[p], out)
	return uint64(p)<<32 | uint64(len(out))
}

//go:wasmexport acton_mailcal_capabilities
func capabilities(p, l uint32) uint64 { return serve(abi.ExportCapabilities, p, l) }

//go:wasmexport acton_mail_list
func mailList(p, l uint32) uint64 { return serve(abi.ExportMailList, p, l) }

//go:wasmexport acton_mail_get
func mailGet(p, l uint32) uint64 { return serve(abi.ExportMailGet, p, l) }

//go:wasmexport acton_mail_get_thread
func mailGetThread(p, l uint32) uint64 { return serve(abi.ExportMailGetThread, p, l) }

//go:wasmexport acton_mail_search
func mailSearch(p, l uint32) uint64 { return serve(abi.ExportMailSearch, p, l) }

//go:wasmexport acton_mail_create_draft
func mailCreateDraft(p, l uint32) uint64 { return serve(abi.ExportMailCreateDraft, p, l) }

//go:wasmexport acton_mail_send
func mailSend(p, l uint32) uint64 { return serve(abi.ExportMailSend, p, l) }

//go:wasmexport acton_mail_mark
func mailMark(p, l uint32) uint64 { return serve(abi.ExportMailMark, p, l) }

//go:wasmexport acton_mail_move
func mailMove(p, l uint32) uint64 { return serve(abi.ExportMailMove, p, l) }

//go:wasmexport acton_mail_delete
func mailDelete(p, l uint32) uint64 { return serve(abi.ExportMailDelete, p, l) }

//go:wasmexport acton_calendar_list_events
func calList(p, l uint32) uint64 { return serve(abi.ExportCalListEvents, p, l) }

//go:wasmexport acton_calendar_get_event
func calGet(p, l uint32) uint64 { return serve(abi.ExportCalGetEvent, p, l) }

//go:wasmexport acton_calendar_create_event
func calCreate(p, l uint32) uint64 { return serve(abi.ExportCalCreateEvent, p, l) }

//go:wasmexport acton_calendar_update_event
func calUpdate(p, l uint32) uint64 { return serve(abi.ExportCalUpdateEvent, p, l) }

//go:wasmexport acton_calendar_respond
func calRespond(p, l uint32) uint64 { return serve(abi.ExportCalRespond, p, l) }

//go:wasmexport acton_calendar_delete_event
func calDelete(p, l uint32) uint64 { return serve(abi.ExportCalDeleteEvent, p, l) }

//go:wasmexport acton_calendar_free_busy
func calFreeBusy(p, l uint32) uint64 { return serve(abi.ExportCalFreeBusy, p, l) }
