package mailcal_test

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/refstore"
	"github.com/actonos/actonos/internal/tools"
)

func run(t *testing.T, fx *fixture, tool string, args any) (*tools.ToolResult, error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	return fx.registry.Execute(context.Background(), "agent_test", tool, raw)
}

func mailcalToolNames(reg *tools.ToolRegistry) []string {
	var names []string
	for _, info := range reg.List() {
		if strings.HasPrefix(info.Name, "native_mail_") || strings.HasPrefix(info.Name, "native_calendar_") || strings.HasPrefix(info.Name, "native_mailcal_") {
			names = append(names, info.Name)
		}
	}
	sort.Strings(names)
	return names
}

// Phase 1 ships reading and drafting only. If a write tool shows up here before the approval
// gate exists (Phase 2), this test fails and the change must not be merged.
func TestOnlyReadAndDraftToolsExistInPhase1(t *testing.T) {
	fx := newFixture(t, nil)
	want := mailcal.ReadToolNames()
	sort.Strings(want)
	if got := mailcalToolNames(fx.registry); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("registered mail/calendar tools = %v, want exactly %v", got, want)
	}
	for _, forbidden := range []string{"native_mail_send", "native_mail_reply", "native_mail_delete", "native_mail_mark", "native_mail_move",
		"native_calendar_create", "native_calendar_update", "native_calendar_respond", "native_calendar_delete", "native_mailcal_propose_rule"} {
		if _, err := fx.registry.Get(forbidden); err == nil {
			t.Errorf("%s must not be registered before the policy gate exists", forbidden)
		}
	}
}

func TestToolsAppearOnlyWhileAnAccountExists(t *testing.T) {
	fx := newFixture(t, nil)
	ctx := context.Background()
	if len(mailcalToolNames(fx.registry)) == 0 {
		t.Fatal("tools should be registered with an account present")
	}
	if err := fx.svc.Accounts().Delete(ctx, fx.account.ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.SyncTools(ctx); err != nil {
		t.Fatal(err)
	}
	if got := mailcalToolNames(fx.registry); len(got) != 0 {
		t.Fatalf("tools must be withdrawn when the last account is deleted, still have %v", got)
	}
	// Idempotent in both directions.
	if err := fx.svc.SyncTools(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMailListWrapsEveryExternalByteAndExplainsWhy(t *testing.T) {
	fx := newFixture(t, nil)
	res, err := run(t, fx, mailcal.ToolMailList, map[string]any{"limit": 10})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(res.Content, "[UNTRUSTED email") != 4 || strings.Count(res.Content, "[/UNTRUSTED email]") != 4 {
		t.Fatalf("every message must be inside its own block:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, mailcal.UntrustedNotice) {
		t.Fatal("the notice telling the model not to obey the content is missing")
	}
	if got := res.Data["count"]; got != 4 {
		t.Fatalf("data count = %v", got)
	}
	for _, v := range res.Data {
		if s, ok := v.(string); ok && strings.Contains(strings.ToLower(s), "ignore all previous") {
			t.Fatalf("attacker text leaked into structured data: %q", s)
		}
	}
}

func TestReadingTheHostileMessageKeepsItInsideOneBlock(t *testing.T) {
	fx := newFixture(t, nil)
	res, err := run(t, fx, mailcal.ToolMailGet, map[string]any{"id": refstore.InjectionMessageID})
	if err != nil {
		t.Fatal(err)
	}
	open := strings.Index(res.Content, "[UNTRUSTED email")
	closeAt := strings.Index(res.Content, "[/UNTRUSTED email]")
	inj := strings.Index(res.Content, "IGNORE ALL PREVIOUS INSTRUCTIONS")
	if open < 0 || closeAt < 0 || inj < open || inj > closeAt {
		t.Fatalf("the injected text must sit strictly inside the untrusted block:\n%s", res.Content)
	}
	if !strings.HasSuffix(strings.TrimSpace(res.Content), mailcal.UntrustedNotice) {
		t.Fatal("the do-not-obey notice must come after the block")
	}
}

func TestMailGetThreadAndSearchAndErrors(t *testing.T) {
	fx := newFixture(t, nil)
	th, err := run(t, fx, mailcal.ToolMailThread, map[string]any{"thread_id": "t1"})
	if err != nil || strings.Count(th.Content, "[UNTRUSTED email") != 3 {
		t.Fatalf("thread: %v\n%v", err, th)
	}
	s, err := run(t, fx, mailcal.ToolMailSearch, map[string]any{"query": "lunch"})
	if err != nil || !strings.Contains(s.Content, "id=\"m2\"") {
		t.Fatalf("search: %v\n%v", err, s)
	}
	for name, args := range map[string]map[string]any{
		mailcal.ToolMailGet:    {"id": "nope"},
		mailcal.ToolMailThread: {"thread_id": "nope"},
		mailcal.ToolMailSearch: {"query": "  "},
		mailcal.ToolMailList:   {"account": "acct_missing"},
	} {
		if _, err := run(t, fx, name, args); err == nil {
			t.Errorf("%s with bad input must fail", name)
		}
	}
}

func TestDraftToolCreatesADraftAndSendsNothing(t *testing.T) {
	provider := mailcal.NewMemProvider(refstore.DefaultSeed())
	fx := newFixture(t, provider)
	res, err := run(t, fx, mailcal.ToolMailDraft, map[string]any{
		"to": []string{"Alice@Partner.com"}, "subject": "Re: Q3 invoice", "body": "Paid.", "in_reply_to": "m5",
	})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.Data["draft_id"].(string)
	if id == "" || !strings.Contains(res.Content, "Nothing has been sent") {
		t.Fatalf("unexpected draft result: %+v", res)
	}
	d, err := fx.svc.Drafts().Get(context.Background(), id)
	if err != nil || d.CreatedBy != "agent_test" || d.Payload.To[0].Email != "alice@partner.com" {
		t.Fatalf("stored draft wrong: %+v %v", d, err)
	}
	sent, err := provider.MailList(context.Background(), fx.account.Ref(), abi.MailListParams{Folder: "Sent"})
	if err != nil || len(sent.Messages) != 1 { // only the seeded m4
		t.Fatalf("a draft must not send anything; Sent holds %d message(s)", len(sent.Messages))
	}
}

func TestDraftToolRejectsBadRecipientsAndInjectionInHeaders(t *testing.T) {
	fx := newFixture(t, nil)
	for name, args := range map[string]map[string]any{
		"two in one":       {"to": []string{"a@b.com, mallory@evil.test"}, "subject": "s", "body": "b"},
		"header injection": {"to": []string{"a@b.com"}, "subject": "s\nBcc: mallory@evil.test", "body": "b"},
		"no recipients":    {"to": []string{}, "subject": "s", "body": "b"},
		"unknown reply":    {"to": []string{"a@b.com"}, "subject": "s", "body": "b", "in_reply_to": "nope"},
	} {
		if _, err := run(t, fx, mailcal.ToolMailDraft, args); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestCalendarTools(t *testing.T) {
	fx := newFixture(t, nil)
	res, err := run(t, fx, mailcal.ToolCalList, map[string]any{"from": "2026-03-03", "to": "2026-03-04"})
	if err != nil || strings.Count(res.Content, "[UNTRUSTED event") != 2 {
		t.Fatalf("calendar list: %v\n%v", err, res)
	}
	got, err := run(t, fx, mailcal.ToolCalGet, map[string]any{"id": "e2"})
	if err != nil || !strings.Contains(got.Content, "Partner call") {
		t.Fatalf("calendar get: %v\n%v", err, got)
	}
	fb, err := run(t, fx, mailcal.ToolCalFreeBusy, map[string]any{"emails": []string{"alice@partner.com"}, "from": "2026-03-03T00:00:00Z", "to": "2026-03-04T00:00:00Z"})
	if err != nil || strings.Count(fb.Content, "busy") != 2 {
		t.Fatalf("free busy: %v\n%v", err, fb)
	}
	for name, args := range map[string]map[string]any{
		mailcal.ToolCalList:     {"from": "yesterday", "to": "2026-03-04"},
		mailcal.ToolCalFreeBusy: {"emails": []string{}, "from": "2026-03-03", "to": "2026-03-04"},
		mailcal.ToolCalGet:      {"id": "nope"},
	} {
		if _, err := run(t, fx, name, args); err == nil {
			t.Errorf("%s with bad input must fail", name)
		}
	}
}
