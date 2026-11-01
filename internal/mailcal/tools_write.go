package mailcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/policy"
	"github.com/actonos/actonos/internal/tools"
)

// Write tool names. All but ToolMailMark/ToolMailMove/ToolProposeRule are in the registry's
// external-effect list and cannot run without the gate installed below.
const (
	ToolMailSend    = "native_mail_send"
	ToolMailDelete  = "native_mail_delete"
	ToolMailMark    = "native_mail_mark"
	ToolMailMove    = "native_mail_move"
	ToolCalCreate   = "native_calendar_create_event"
	ToolCalUpdate   = "native_calendar_update_event"
	ToolCalRespond  = "native_calendar_respond"
	ToolCalDelete   = "native_calendar_delete_event"
	ToolProposeRule = "native_mailcal_propose_rule"
)

// GatedToolNames are the tools the gate covers.
func GatedToolNames() []string {
	return []string{ToolMailSend, ToolMailDelete, ToolCalCreate, ToolCalUpdate, ToolCalRespond, ToolCalDelete}
}

// WriteToolNames lists every tool Phase 2 adds.
func WriteToolNames() []string {
	return append(GatedToolNames(), ToolMailMark, ToolMailMove, ToolProposeRule)
}

type eventInput struct {
	Account     string   `json:"account"`
	ID          string   `json:"id"`
	CalendarID  string   `json:"calendar_id"`
	Title       string   `json:"title"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	TimeZone    string   `json:"time_zone"`
	Location    string   `json:"location"`
	Description string   `json:"description"`
	Attendees   []string `json:"attendees"`
	SendUpdates bool     `json:"send_updates"`
}

func (in eventInput) event() (abi.Event, error) {
	start, err := parseTime("start", in.Start)
	if err != nil {
		return abi.Event{}, err
	}
	end, err := parseTime("end", in.End)
	if err != nil {
		return abi.Event{}, err
	}
	ev := abi.Event{ID: in.ID, CalendarID: in.CalendarID, Title: strings.TrimSpace(in.Title), Start: start, End: end,
		TimeZone: in.TimeZone, Location: in.Location, Description: in.Description}
	for _, e := range in.Attendees {
		n, err := abi.NormalizeEmail(e)
		if err != nil {
			return abi.Event{}, fmt.Errorf("attendee %q: %w", e, err)
		}
		ev.Attendees = append(ev.Attendees, abi.Attendee{Email: n})
	}
	if ev.Title == "" {
		return abi.Event{}, errors.New("title is required")
	}
	return ev, nil
}

type respondInput struct {
	Account    string `json:"account"`
	ID         string `json:"id"`
	CalendarID string `json:"calendar_id"`
	Response   string `json:"response"`
	Comment    string `json:"comment"`
}

func (in respondInput) params() (abi.CalRespondParams, error) {
	switch in.Response {
	case "accepted", "declined", "tentative":
	default:
		return abi.CalRespondParams{}, errors.New("response must be accepted, declined or tentative")
	}
	return abi.CalRespondParams{CalendarID: in.CalendarID, ID: in.ID, Response: in.Response, Comment: in.Comment}, nil
}

func (s *Service) opts(ctx context.Context) WriteOptions {
	agent := tools.AgentIDFromContext(ctx)
	if agent == "" {
		agent = "unknown"
	}
	return WriteOptions{AgentID: agent, TraceID: tools.TraceIDFromContext(ctx), Approved: tools.ApprovalIDFromContext(ctx) != ""}
}

const eventProps = accountProp + `,"calendar_id":{"type":"string"},"title":{"type":"string"},` +
	`"start":{"type":"string","description":"RFC 3339"},"end":{"type":"string","description":"RFC 3339"},` +
	`"time_zone":{"type":"string","description":"IANA zone of the event, e.g. Asia/Ho_Chi_Minh"},` +
	`"location":{"type":"string"},"description":{"type":"string"},"attendees":{"type":"array","items":{"type":"string"}},` +
	`"send_updates":{"type":"boolean"}`

func (s *Service) writeTools() []tools.Tool {
	return []tools.Tool{
		&fnTool{name: ToolMailSend,
			description: "Send a draft created with native_mail_draft. Only the draft id is accepted. Without an approved rule a person must approve first, so expect an approval request.",
			schema:      schema(`"draft_id":{"type":"string"}`, "draft_id"), run: s.toolMailSend},
		&fnTool{name: ToolMailDelete, description: "Delete emails. Always needs a person's approval.",
			schema: schema(accountProp+`,"ids":{"type":"array","items":{"type":"string"}}`, "ids"), run: s.toolMailDelete},
		&fnTool{name: ToolMailMark, description: "Mark emails read or unread.",
			schema: schema(accountProp+`,"ids":{"type":"array","items":{"type":"string"}},"unread":{"type":"boolean"}`, "ids", "unread"), run: s.toolMailMark},
		&fnTool{name: ToolMailMove, description: "Move emails to a folder or label.",
			schema: schema(accountProp+`,"ids":{"type":"array","items":{"type":"string"}},"folder":{"type":"string"}`, "ids", "folder"), run: s.toolMailMove},
		&fnTool{name: ToolCalCreate, description: "Create a calendar event. Needs approval unless a rule allows it.",
			schema: schema(eventProps, "title", "start", "end"), run: s.toolCalCreate},
		&fnTool{name: ToolCalUpdate, description: "Replace a calendar event. Needs approval unless a rule allows it.",
			schema: schema(eventProps+`,"id":{"type":"string"}`, "id", "title", "start", "end"), run: s.toolCalUpdate},
		&fnTool{name: ToolCalRespond, description: "Accept, decline or tentatively accept an invitation.",
			schema: schema(accountProp+`,"id":{"type":"string"},"calendar_id":{"type":"string"},"response":{"type":"string","enum":["accepted","declined","tentative"]},"comment":{"type":"string"}`,
				"id", "response"), run: s.toolCalRespond},
		&fnTool{name: ToolCalDelete, description: "Delete a calendar event. Always needs a person's approval.",
			schema: schema(accountProp+`,"id":{"type":"string"},"calendar_id":{"type":"string"},"send_updates":{"type":"boolean"}`, "id"), run: s.toolCalDelete},
		&fnTool{name: ToolProposeRule,
			description: "Suggest an automatic-send rule to the owner. It has NO effect until the owner approves it in the app. Be as narrow as possible.",
			schema:      schema(`"rule":{"type":"object","description":"A rule: account_id, action, effect, match, limits"},"rationale":{"type":"string"}`, "rule"),
			run:         s.toolProposeRule},
	}
}

func (s *Service) toolMailSend(ctx context.Context, input json.RawMessage) (*tools.ToolResult, error) {
	var in struct {
		DraftID string `json:"draft_id"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	res, err := s.Send(ctx, strings.TrimSpace(in.DraftID), s.opts(ctx))
	if err != nil {
		return nil, err
	}
	msg := "Sent."
	if res.Replayed {
		msg = "This draft was already sent earlier; nothing new was sent."
	}
	return &tools.ToolResult{Content: fmt.Sprintf("%s\nprovider_message_id: %s", msg, res.ProviderMessageID),
		Data: map[string]any{"provider_message_id": res.ProviderMessageID, "replayed": res.Replayed}}, nil
}

func (s *Service) toolMailDelete(ctx context.Context, input json.RawMessage) (*tools.ToolResult, error) {
	var in struct {
		Account string   `json:"account"`
		IDs     []string `json:"ids"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	if err := s.MailDelete(ctx, in.Account, in.IDs, s.opts(ctx)); err != nil {
		return nil, err
	}
	return &tools.ToolResult{Content: fmt.Sprintf("Deleted %d message(s).", len(in.IDs))}, nil
}

func (s *Service) simpleMailWrite(ctx context.Context, account, export string, call func(b binding) error) error {
	b, err := s.resolve(ctx, account, KindMail)
	if err != nil {
		return err
	}
	if err := b.require(export); err != nil {
		return err
	}
	if !s.settings.Enabled(ctx) {
		return ErrMailcalOff
	}
	return s.writeCall(ctx, b, func() error { return call(b) })
}

func (s *Service) toolMailMark(ctx context.Context, input json.RawMessage) (*tools.ToolResult, error) {
	var in struct {
		Account string   `json:"account"`
		IDs     []string `json:"ids"`
		Unread  bool     `json:"unread"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	if len(in.IDs) == 0 {
		return nil, errors.New("no message ids given")
	}
	if err := s.simpleMailWrite(ctx, in.Account, abi.ExportMailMark, func(b binding) error {
		return b.provider.MailMark(ctx, b.account.Ref(), abi.MailMarkParams{IDs: in.IDs, Unread: in.Unread})
	}); err != nil {
		return nil, err
	}
	return &tools.ToolResult{Content: fmt.Sprintf("Marked %d message(s).", len(in.IDs))}, nil
}

func (s *Service) toolMailMove(ctx context.Context, input json.RawMessage) (*tools.ToolResult, error) {
	var in struct {
		Account string   `json:"account"`
		IDs     []string `json:"ids"`
		Folder  string   `json:"folder"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	if len(in.IDs) == 0 || strings.TrimSpace(in.Folder) == "" {
		return nil, errors.New("ids and folder are required")
	}
	if err := s.simpleMailWrite(ctx, in.Account, abi.ExportMailMove, func(b binding) error {
		return b.provider.MailMove(ctx, b.account.Ref(), abi.MailMoveParams{IDs: in.IDs, Folder: in.Folder})
	}); err != nil {
		return nil, err
	}
	return &tools.ToolResult{Content: fmt.Sprintf("Moved %d message(s).", len(in.IDs))}, nil
}

func (s *Service) toolCalCreate(ctx context.Context, input json.RawMessage) (*tools.ToolResult, error) {
	var in eventInput
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	ev, err := in.event()
	if err != nil {
		return nil, err
	}
	out, err := s.CalendarCreate(ctx, in.Account, ev, in.SendUpdates, s.opts(ctx))
	if err != nil {
		return nil, err
	}
	return &tools.ToolResult{Content: "Event created.\n" + renderEvent(out), Data: map[string]any{"event_id": out.ID}}, nil
}

func (s *Service) toolCalUpdate(ctx context.Context, input json.RawMessage) (*tools.ToolResult, error) {
	var in eventInput
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	ev, err := in.event()
	if err != nil {
		return nil, err
	}
	out, err := s.CalendarUpdate(ctx, in.Account, ev, in.SendUpdates, s.opts(ctx))
	if err != nil {
		return nil, err
	}
	return &tools.ToolResult{Content: "Event updated.\n" + renderEvent(out), Data: map[string]any{"event_id": out.ID}}, nil
}

func (s *Service) toolCalRespond(ctx context.Context, input json.RawMessage) (*tools.ToolResult, error) {
	var in respondInput
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	p, err := in.params()
	if err != nil {
		return nil, err
	}
	if _, err := s.CalendarRespond(ctx, in.Account, p, s.opts(ctx)); err != nil {
		return nil, err
	}
	return &tools.ToolResult{Content: "Response sent: " + p.Response}, nil
}

func (s *Service) toolCalDelete(ctx context.Context, input json.RawMessage) (*tools.ToolResult, error) {
	var in struct {
		Account     string `json:"account"`
		ID          string `json:"id"`
		CalendarID  string `json:"calendar_id"`
		SendUpdates bool   `json:"send_updates"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	if err := s.CalendarDelete(ctx, in.Account, abi.CalDeleteParams{CalendarID: in.CalendarID, ID: in.ID, SendUpdates: in.SendUpdates}, s.opts(ctx)); err != nil {
		return nil, err
	}
	return &tools.ToolResult{Content: "Event deleted."}, nil
}

// toolProposeRule only writes a pending, disabled proposal. It has no access to the rule book.
func (s *Service) toolProposeRule(ctx context.Context, input json.RawMessage) (*tools.ToolResult, error) {
	var in struct {
		Rule      policy.Rule `json:"rule"`
		Rationale string      `json:"rationale"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	agent := tools.AgentIDFromContext(ctx)
	p, err := s.proposals.Propose(ctx, SourceAgent, agent, in.Rule, in.Rationale)
	if err != nil {
		return nil, err
	}
	return &tools.ToolResult{
		Content: fmt.Sprintf("Proposal %s recorded. It does nothing until the owner approves it in the app.", p.ID),
		Data:    map[string]any{"proposal_id": p.ID},
	}, nil
}

// Check implements tools.ToolGate for the external-effect tools. It evaluates the same
// request the tool will evaluate again when it runs, and writes an audit line.
func (s *Service) Check(ctx context.Context, agentID, toolName string, input json.RawMessage) (tools.GateDecision, error) {
	req, draft, err := s.gateRequest(ctx, toolName, input)
	if err != nil {
		return tools.GateDecision{}, err
	}
	dec := s.Decide(ctx, req)
	_ = s.decisions.Record(context.WithoutCancel(ctx), DecisionRecord{
		Phase: PhaseGate, AccountID: req.AccountID, AgentID: agentID, Tool: toolName, DraftID: draft.ID, PayloadHash: draft.PayloadHash,
		Verdict: dec.Verdict, RuleID: dec.RuleID, Reasons: dec.Reasons,
	})
	reason := strings.Join(dec.Reasons, "; ")
	switch dec.Verdict {
	case policy.VerdictAllow:
		return tools.GateDecision{Action: tools.GateAllow, Reason: reason}, nil
	case policy.VerdictNeedsApproval:
		return tools.GateDecision{Action: tools.GateRequireApproval, Reason: reason}, nil
	default:
		return tools.GateDecision{Action: tools.GateDeny, Reason: reason}, nil
	}
}

func (s *Service) gateRequest(ctx context.Context, toolName string, input json.RawMessage) (policy.Request, Draft, error) {
	switch toolName {
	case ToolMailSend:
		var in struct {
			DraftID string `json:"draft_id"`
		}
		if err := decodeInput(input, &in); err != nil {
			return policy.Request{}, Draft{}, err
		}
		d, err := s.drafts.Get(ctx, strings.TrimSpace(in.DraftID))
		if err != nil {
			return policy.Request{}, Draft{}, err
		}
		return requestForDraft(d), d, nil
	case ToolMailDelete:
		var in struct {
			Account string `json:"account"`
		}
		if err := decodeInput(input, &in); err != nil {
			return policy.Request{}, Draft{}, err
		}
		b, err := s.resolve(ctx, in.Account, KindMail)
		if err != nil {
			return policy.Request{}, Draft{}, err
		}
		return policy.Request{AccountID: b.account.ID, Action: actionMailDelete, Destructive: true}, Draft{}, nil
	case ToolCalCreate, ToolCalUpdate:
		var in eventInput
		if err := decodeInput(input, &in); err != nil {
			return policy.Request{}, Draft{}, err
		}
		ev, err := in.event()
		if err != nil {
			return policy.Request{}, Draft{}, err
		}
		b, err := s.resolve(ctx, in.Account, KindCalendar)
		if err != nil {
			return policy.Request{}, Draft{}, err
		}
		facts, err := eventFacts(ev)
		if err != nil {
			return policy.Request{}, Draft{}, err
		}
		action := policy.ActionCalendarCreate
		if toolName == ToolCalUpdate {
			action = policy.ActionCalendarUpdate
		}
		return policy.Request{AccountID: b.account.ID, Action: action, Calendar: facts}, Draft{}, nil
	case ToolCalRespond:
		var in respondInput
		if err := decodeInput(input, &in); err != nil {
			return policy.Request{}, Draft{}, err
		}
		p, err := in.params()
		if err != nil {
			return policy.Request{}, Draft{}, err
		}
		b, err := s.resolve(ctx, in.Account, KindCalendar)
		if err != nil {
			return policy.Request{}, Draft{}, err
		}
		req, err := s.respondRequest(ctx, b, p)
		return req, Draft{}, err
	case ToolCalDelete:
		var in struct {
			Account string `json:"account"`
		}
		if err := decodeInput(input, &in); err != nil {
			return policy.Request{}, Draft{}, err
		}
		b, err := s.resolve(ctx, in.Account, KindCalendar)
		if err != nil {
			return policy.Request{}, Draft{}, err
		}
		return policy.Request{AccountID: b.account.ID, Action: actionCalDelete, Destructive: true}, Draft{}, nil
	}
	return policy.Request{}, Draft{}, fmt.Errorf("no gate rules for tool %q", toolName)
}

var _ tools.ToolGate = (*Service)(nil)
var _ = time.Second
