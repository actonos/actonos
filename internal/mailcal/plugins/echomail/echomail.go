// Package echomail is the smallest useful provider plugin: a read-only mailbox whose messages are
// written in the account's config ({"messages": [...]}). It is kept as the worked example for
// docs/MAILCAL-PLUGIN-GUIDE.md and proves that adding a provider touches no file outside its own
// directory.
package echomail

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/provkit"
)

// Backend serves messages from the account config. Everything it does not override answers
// "unsupported" through provkit.Base.
type Backend struct{ provkit.Base }

// Capabilities declares exactly the operations implemented below; the conformance suite checks
// that anything not listed here really answers "unsupported".
func (Backend) Capabilities(context.Context, abi.AccountRef) (abi.Capabilities, error) {
	return abi.Capabilities{
		ABI:   []string{abi.CapMail},
		Ops:   []string{abi.ExportCapabilities, abi.ExportMailList, abi.ExportMailSearch, abi.ExportMailGet, abi.ExportMailGetThread},
		Notes: "read-only example provider",
	}, nil
}

func messages(acct abi.AccountRef) ([]abi.Message, error) {
	raw, ok := acct.Config["messages"]
	if !ok {
		return nil, nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, abi.Errorf(abi.ErrInvalid, "config.messages must be a list of messages")
	}
	var out []abi.Message
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, abi.Errorf(abi.ErrInvalid, "config.messages must be a list of messages: %v", err)
	}
	return out, nil
}

// withoutBody is what list results look like: the body is only returned by MailGet.
func withoutBody(m abi.Message) abi.Message {
	m.BodyText = ""
	return m
}

func page(all []abi.Message, limit int, keep func(abi.Message) bool) abi.MailListResult {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	res := abi.MailListResult{Messages: []abi.Message{}}
	for _, m := range all {
		if len(res.Messages) == limit {
			break
		}
		if keep(m) {
			res.Messages = append(res.Messages, withoutBody(m))
		}
	}
	return res
}

func (Backend) MailList(_ context.Context, acct abi.AccountRef, p abi.MailListParams) (abi.MailListResult, error) {
	all, err := messages(acct)
	if err != nil {
		return abi.MailListResult{}, err
	}
	folder := p.Folder
	if folder == "" {
		folder = "INBOX"
	}
	return page(all, p.Limit, func(m abi.Message) bool {
		return (m.Folder == "" && folder == "INBOX" || m.Folder == folder) && (!p.UnreadOnly || m.Unread)
	}), nil
}

func (Backend) MailSearch(_ context.Context, acct abi.AccountRef, p abi.MailSearchParams) (abi.MailListResult, error) {
	all, err := messages(acct)
	if err != nil {
		return abi.MailListResult{}, err
	}
	q := strings.ToLower(strings.TrimSpace(p.Query))
	return page(all, p.Limit, func(m abi.Message) bool {
		return q != "" && strings.Contains(strings.ToLower(m.Subject+" "+m.BodyText+" "+m.From.Email), q)
	}), nil
}

func (Backend) MailGet(_ context.Context, acct abi.AccountRef, id string) (abi.Message, error) {
	all, err := messages(acct)
	if err != nil {
		return abi.Message{}, err
	}
	for _, m := range all {
		if m.ID == id {
			return m, nil
		}
	}
	return abi.Message{}, abi.Errorf(abi.ErrNotFound, "no message %q", id)
}

func (Backend) MailGetThread(_ context.Context, acct abi.AccountRef, threadID string) (abi.Thread, error) {
	all, err := messages(acct)
	if err != nil {
		return abi.Thread{}, err
	}
	t := abi.Thread{ID: threadID}
	for _, m := range all {
		if m.ThreadID == threadID {
			t.Messages = append(t.Messages, m)
		}
	}
	if len(t.Messages) == 0 {
		return abi.Thread{}, abi.Errorf(abi.ErrNotFound, "no thread %q", threadID)
	}
	return t, nil
}
