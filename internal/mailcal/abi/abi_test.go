package abi

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata golden files")

// goldenSample populates every field of every wire type once. If someone renames or
// removes a field, the marshalled JSON changes and TestGoldenV1 fails. Adding a new
// optional field is allowed: regenerate with -update and review the diff (additions only).
func goldenSample() map[string]any {
	ts := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	addr := Address{Name: "Alice", Email: "alice@example.com"}
	msg := Message{
		ID: "m1", ThreadID: "t1", Folder: "INBOX", From: addr, To: []Address{addr}, Cc: []Address{addr},
		Subject: "Hello", Date: ts, Snippet: "Hi", BodyText: "Hi there", Labels: []string{"x"}, Unread: true,
		InReplyTo: "m0", Attachments: []AttachmentMeta{{ID: "a1", Name: "f.pdf", MIME: "application/pdf", Size: 10}},
	}
	ev := Event{
		ID: "e1", CalendarID: "primary", Title: "Sync", Start: ts, End: ts.Add(time.Hour), TimeZone: "UTC",
		AllDay: true, Location: "Room", Description: "d", ConferenceURL: "https://meet.example.com/x",
		Attendees: []Attendee{{Email: "bob@example.com", Name: "Bob", Status: "accepted"}}, Organizer: &addr,
	}
	return map[string]any{
		"request":            Request{Account: AccountRef{ID: "acct1", Config: map[string]any{"host": "h"}}, Params: json.RawMessage(`{"k":1}`)},
		"response_ok":        Response{Result: json.RawMessage(`{"ok":true}`)},
		"response_err":       Response{Error: &Error{Code: ErrRateLimited, Message: "slow down", RetryAfterMS: 500}},
		"capabilities":       Capabilities{ABI: []string{CapMail, CapCalendar}, Ops: AllExports, Limits: Limits{MaxRecipients: 50, MaxBodyBytes: 1000, MaxAttachmentBytes: 5}, Notes: "n"},
		"message":            msg,
		"thread":             Thread{ID: "t1", Messages: []Message{msg}},
		"mail_list_params":   MailListParams{Folder: "INBOX", Cursor: "c", Limit: 10, UnreadOnly: true},
		"mail_search_params": MailSearchParams{Query: "invoice", Folder: "INBOX", Cursor: "c", Limit: 10},
		"mail_list_result":   MailListResult{Messages: []Message{msg}, NextCursor: "n"},
		"mail_get_params":    MailGetParams{ID: "m1"},
		"mail_thread_params": MailGetThreadParams{ThreadID: "t1"},
		"mail_send_params": MailSendParams{
			To: []Address{addr}, Cc: []Address{addr}, Bcc: []Address{addr}, Subject: "s", BodyText: "b",
			InReplyTo: "m1", ThreadID: "t1", IdempotencyKey: "k",
		},
		"mail_send_result":   MailSendResult{ProviderMessageID: "p1", ThreadID: "t1", Replayed: true},
		"mail_draft_params":  MailDraftParams{To: []Address{addr}, Cc: []Address{addr}, Subject: "s", BodyText: "b", InReplyTo: "m1"},
		"mail_draft_result":  MailDraftResult{ProviderDraftID: "d1"},
		"mail_mark_params":   MailMarkParams{IDs: []string{"m1"}, Unread: true},
		"mail_move_params":   MailMoveParams{IDs: []string{"m1"}, Folder: "Archive"},
		"mail_delete_params": MailDeleteParams{IDs: []string{"m1"}},
		"event":              ev,
		"cal_list_params":    CalListParams{CalendarID: "primary", From: ts, To: ts.Add(24 * time.Hour), Cursor: "c", Limit: 5},
		"cal_list_result":    CalListResult{Events: []Event{ev}, NextCursor: "n"},
		"cal_get_params":     CalGetParams{CalendarID: "primary", ID: "e1"},
		"cal_write_params":   CalWriteParams{Event: ev, SendUpdates: true, IdempotencyKey: "k"},
		"cal_respond":        CalRespondParams{CalendarID: "primary", ID: "e1", Response: "accepted", Comment: "ok"},
		"cal_delete":         CalDeleteParams{CalendarID: "primary", ID: "e1", SendUpdates: true},
		"free_busy_params":   FreeBusyParams{Emails: []string{"alice@example.com"}, From: ts, To: ts.Add(time.Hour)},
		"free_busy_result":   FreeBusyResult{Busy: map[string][]BusyInterval{"alice@example.com": {{Start: ts, End: ts.Add(time.Hour)}}}},
	}
}

func TestGoldenV1(t *testing.T) {
	got, err := json.MarshalIndent(goldenSample(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "golden_v1.json")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file (run: go test ./internal/mailcal/abi -update): %v", err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("wire format changed. The ABI is additive-only: restore the old fields or, if you only added optional fields, regenerate with -update and check the diff has additions only.\n--- got ---\n%s", got)
	}
}

// Old hosts and new guests (and vice versa) must tolerate fields they do not know.
func TestUnknownFieldsAreIgnored(t *testing.T) {
	raw := `{"id":"m1","thread_id":"t1","from":{"email":"a@b.com","future":1},"subject":"s","date":"2026-03-01T09:30:00Z","brand_new_field":{"x":1}}`
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unknown fields must not break decoding: %v", err)
	}
	if m.ID != "m1" || m.From.Email != "a@b.com" {
		t.Fatalf("known fields lost: %+v", m)
	}
}

func TestAllExportsAreUniqueAndPrefixed(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range AllExports {
		if seen[name] {
			t.Errorf("duplicate export %q", name)
		}
		seen[name] = true
		if len(name) < 6 || name[:6] != "acton_" {
			t.Errorf("export %q must start with acton_", name)
		}
	}
}

func TestErrorImplementsErrorAndRoundTrips(t *testing.T) {
	var err error = Errorf(ErrNotFound, "message %s", "m9")
	if err.Error() != "not_found: message m9" {
		t.Fatalf("unexpected text %q", err.Error())
	}
	raw, _ := json.Marshal(Response{Error: &Error{Code: ErrAuthExpired}})
	var back Response
	if json.Unmarshal(raw, &back) != nil || back.Error == nil || back.Error.Code != ErrAuthExpired || back.Error.Error() != "auth_expired" {
		t.Fatalf("round trip failed: %s", raw)
	}
}

func TestCapabilitiesSupports(t *testing.T) {
	c := Capabilities{Ops: []string{ExportMailList}}
	if !c.Supports(ExportMailList) || c.Supports(ExportMailSend) {
		t.Fatal("Supports misreports")
	}
}
