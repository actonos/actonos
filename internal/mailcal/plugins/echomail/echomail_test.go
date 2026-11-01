package echomail

import (
	"context"
	"testing"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

func acct() abi.AccountRef {
	return abi.AccountRef{ID: "a", Config: map[string]any{"messages": []any{
		map[string]any{"id": "1", "thread_id": "t1", "from": map[string]any{"email": "a@example.com"}, "subject": "Hello", "body_text": "first body", "unread": true},
		map[string]any{"id": "2", "thread_id": "t1", "from": map[string]any{"email": "b@example.com"}, "subject": "Re: Hello", "body_text": "second body"},
		map[string]any{"id": "3", "thread_id": "t2", "folder": "Archive", "from": map[string]any{"email": "c@example.com"}, "subject": "Old", "body_text": "old body"},
	}}}
}

func TestListHidesBodiesAndHonoursFolderAndUnread(t *testing.T) {
	var b Backend
	res, err := b.MailList(context.Background(), acct(), abi.MailListParams{})
	if err != nil || len(res.Messages) != 2 || res.Messages[0].BodyText != "" {
		t.Fatalf("%+v %v", res, err)
	}
	res, _ = b.MailList(context.Background(), acct(), abi.MailListParams{UnreadOnly: true})
	if len(res.Messages) != 1 || res.Messages[0].ID != "1" {
		t.Fatalf("unread: %+v", res)
	}
	res, _ = b.MailList(context.Background(), acct(), abi.MailListParams{Folder: "Archive"})
	if len(res.Messages) != 1 || res.Messages[0].ID != "3" {
		t.Fatalf("archive: %+v", res)
	}
}

func TestGetAndThreadAndNotFound(t *testing.T) {
	var b Backend
	m, err := b.MailGet(context.Background(), acct(), "2")
	if err != nil || m.BodyText != "second body" {
		t.Fatalf("%+v %v", m, err)
	}
	th, err := b.MailGetThread(context.Background(), acct(), "t1")
	if err != nil || len(th.Messages) != 2 {
		t.Fatalf("%+v %v", th, err)
	}
	if _, err := b.MailGet(context.Background(), acct(), "nope"); err == nil || err.(*abi.Error).Code != abi.ErrNotFound {
		t.Fatalf("want not_found, got %v", err)
	}
}

func TestWritesAreUnsupportedAndBadConfigIsInvalid(t *testing.T) {
	var b Backend
	if _, err := b.MailSend(context.Background(), acct(), abi.MailSendParams{}); err == nil || err.(*abi.Error).Code != abi.ErrUnsupported {
		t.Fatalf("send must be unsupported: %v", err)
	}
	bad := abi.AccountRef{ID: "x", Config: map[string]any{"messages": "not a list"}}
	if _, err := b.MailList(context.Background(), bad, abi.MailListParams{}); err == nil || err.(*abi.Error).Code != abi.ErrInvalid {
		t.Fatalf("bad config must be invalid: %v", err)
	}
}
