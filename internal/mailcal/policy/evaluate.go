package policy

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

// Verdict is the engine's answer.
type Verdict string

const (
	VerdictAllow         Verdict = "allow"
	VerdictNeedsApproval Verdict = "needs_approval"
	VerdictDeny          Verdict = "deny"
)

// Request is the exact payload the core is about to hand to a provider.
type Request struct {
	AccountID   string
	Action      Action
	Destructive bool // delete operations: never auto-allowed
	To, Cc, Bcc []string
	Subject     string
	Body        string
	Attachments int
	Reply       *ReplyFacts
	Calendar    *CalendarFacts
}

// ReplyFacts come from the provider at draft time, not from the agent.
type ReplyFacts struct {
	Verified     bool
	Participants []string
}

// CalendarFacts describe an event write.
type CalendarFacts struct {
	Start, End time.Time // must carry the event's own time zone
	Attendees  []string
	Organizer  string
}

// Usage returns how many times ruleID has fired in the trailing window. An error means
// "unknown", which the evaluator treats as over the limit.
type Usage func(ruleID string, window time.Duration) (int, error)

// Decision is the verdict plus the reasons, suitable for audit and the approval card.
type Decision struct {
	Verdict Verdict  `json:"verdict"`
	RuleID  string   `json:"rule_id,omitempty"`
	Reasons []string `json:"reasons"`
}

var (
	linkPattern    = regexp.MustCompile(`(?i)(https?://|www\.|mailto:|ftp://|[a-z0-9-]+\.[a-z]{2,}/)`)
	forwardSubject = regexp.MustCompile(`(?i)^\s*(fwd?|tr|wg)\s*:`)
	forwardBody    = regexp.MustCompile(`(?i)(-{3,}\s*forwarded message|begin forwarded message|original message\s*-{3,})`)
)

func needsApproval(reasons ...string) Decision {
	return Decision{Verdict: VerdictNeedsApproval, Reasons: reasons}
}

// Evaluate applies the decision algorithm of the design (section 6). It never returns
// VerdictAllow unless one enabled allow rule matches every constraint and has headroom.
func Evaluate(rules []Rule, in Request, now time.Time, usage Usage) (d Decision) {
	defer func() {
		if r := recover(); r != nil {
			d = needsApproval(fmt.Sprintf("policy engine error: %v", r))
		}
	}()
	req, err := normalizeRequest(in)
	if err != nil {
		return needsApproval("could not read the request safely: " + err.Error())
	}

	candidates := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled || r.AccountID != req.AccountID || r.Action != req.Action {
			continue
		}
		if r.ExpiresAt != nil && !now.Before(*r.ExpiresAt) {
			continue
		}
		candidates = append(candidates, r)
	}

	for _, r := range candidates {
		if r.Effect == EffectDeny && denyMatches(r, req) {
			return Decision{Verdict: VerdictDeny, RuleID: r.ID, Reasons: []string{"blocked by deny rule " + r.ID}}
		}
	}

	if reasons := alwaysApprove(req); len(reasons) > 0 {
		return needsApproval(reasons...)
	}

	var why []string
	for _, r := range candidates {
		if r.Effect != EffectAllow {
			continue
		}
		if reasons := allowMismatch(r, req); len(reasons) > 0 {
			why = append(why, fmt.Sprintf("rule %s: %s", r.ID, strings.Join(reasons, "; ")))
			continue
		}
		if over := overLimit(r, usage, now); over != "" {
			why = append(why, fmt.Sprintf("rule %s: %s", r.ID, over))
			continue
		}
		return Decision{Verdict: VerdictAllow, RuleID: r.ID, Reasons: []string{"matched rule " + r.ID}}
	}
	if len(why) == 0 {
		why = append(why, "no rule allows this action")
	}
	return needsApproval(why...)
}

func normalizeRequest(in Request) (Request, error) {
	out := in
	var err error
	if out.To, err = normList(in.To); err != nil {
		return Request{}, err
	}
	if out.Cc, err = normList(in.Cc); err != nil {
		return Request{}, err
	}
	if out.Bcc, err = normList(in.Bcc); err != nil {
		return Request{}, err
	}
	if in.Reply != nil {
		rf := *in.Reply
		if rf.Participants, err = normList(in.Reply.Participants); err != nil {
			return Request{}, err
		}
		out.Reply = &rf
	}
	if in.Calendar != nil {
		cf := *in.Calendar
		if cf.Attendees, err = normList(in.Calendar.Attendees); err != nil {
			return Request{}, err
		}
		if in.Calendar.Organizer != "" {
			if cf.Organizer, err = abi.NormalizeEmail(in.Calendar.Organizer); err != nil {
				return Request{}, err
			}
		}
		out.Calendar = &cf
	}
	return out, nil
}

func normList(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		e, err := abi.NormalizeEmail(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func isMail(a Action) bool { return a == ActionReply || a == ActionSendNew }

func people(req Request) []string {
	if isMail(req.Action) {
		return append(append([]string{}, req.To...), req.Cc...)
	}
	if req.Calendar != nil {
		return req.Calendar.Attendees
	}
	return nil
}

func listed(email string, addrs, domains []string) bool {
	for _, a := range addrs {
		if a == email {
			return true
		}
	}
	d := abi.DomainOf(email)
	for _, dom := range domains {
		if dom == d {
			return true
		}
	}
	return false
}

// denyMatches is true when the rule names any person in the request (or names nobody,
// which makes it a blanket block for the action).
func denyMatches(r Rule, req Request) bool {
	addrs, domains := r.Match.Recipients.Addresses, r.Match.Recipients.Domains
	if len(addrs)+len(domains) == 0 {
		return true
	}
	everyone := append(people(req), req.Bcc...)
	for _, e := range everyone {
		if listed(e, addrs, domains) {
			return true
		}
	}
	return false
}

func alwaysApprove(req Request) []string {
	var reasons []string
	if req.Destructive {
		reasons = append(reasons, "deleting is always reviewed by a person")
	}
	if len(req.Bcc) > 0 {
		reasons = append(reasons, "the message has Bcc recipients")
	}
	if req.Attachments > 0 {
		reasons = append(reasons, "the message has attachments")
	}
	if isMail(req.Action) && (forwardSubject.MatchString(req.Subject) || forwardBody.MatchString(req.Body)) {
		reasons = append(reasons, "the message looks like a forward")
	}
	return reasons
}

func allowMismatch(r Rule, req Request) []string {
	var reasons []string
	if isMail(req.Action) {
		reasons = append(reasons, mailMismatch(r, req)...)
	} else {
		reasons = append(reasons, calendarMismatch(r, req)...)
	}
	return reasons
}

func mailMismatch(r Rule, req Request) []string {
	var reasons []string
	all := people(req)
	if len(all) == 0 {
		return []string{"no recipients"}
	}
	for _, e := range all {
		if !listed(e, r.Match.Recipients.Addresses, r.Match.Recipients.Domains) {
			reasons = append(reasons, e+" is not covered by the rule")
		}
	}
	if r.Match.ReplyOnly || req.Action == ActionReply {
		switch {
		case req.Reply == nil || !req.Reply.Verified:
			reasons = append(reasons, "this is not a verified reply")
		default:
			for _, e := range all {
				if !contains(req.Reply.Participants, e) {
					reasons = append(reasons, e+" has not written in this thread")
				}
			}
		}
	}
	switch r.Match.Body.Mode {
	case BodyTemplate:
		if normalizeText(req.Body) != normalizeText(r.Match.Body.Template) {
			reasons = append(reasons, "the text differs from the approved template")
		}
	case BodyFree:
	default:
		reasons = append(reasons, "the rule has no valid body mode")
	}
	if !r.Match.Body.AllowLinks && linkPattern.MatchString(req.Body) {
		reasons = append(reasons, "the text contains a link")
	}
	return reasons
}

func calendarMismatch(r Rule, req Request) []string {
	c, f := r.Match.Calendar, req.Calendar
	if c == nil || f == nil {
		return []string{"calendar facts are missing"}
	}
	var reasons []string
	check := f.Attendees
	if req.Action == ActionCalendarAccept {
		check = []string{f.Organizer}
		if f.Organizer == "" {
			return []string{"the organizer is unknown"}
		}
	}
	for _, e := range check {
		if !listed(e, nil, c.Domains) {
			reasons = append(reasons, e+" is outside the allowed domains")
		}
	}
	if req.Action == ActionCalendarAccept {
		return reasons
	}
	if !f.End.After(f.Start) {
		return append(reasons, "the event ends before it starts")
	}
	if mins := int(f.End.Sub(f.Start) / time.Minute); mins > c.MaxMinutes {
		reasons = append(reasons, fmt.Sprintf("the event lasts %d minutes, more than %d", mins, c.MaxMinutes))
	}
	if c.WorkingHours != "" {
		start, end, err := parseHours(c.WorkingHours)
		if err != nil {
			return append(reasons, "invalid working hours")
		}
		sy, sm, sd := f.Start.Date()
		ey, em, ed := f.End.Date()
		s := f.Start.Hour()*60 + f.Start.Minute()
		e := f.End.Hour()*60 + f.End.Minute()
		if sy != ey || sm != em || sd != ed || s < start || e > end {
			reasons = append(reasons, "the event is outside working hours")
		}
	}
	return reasons
}

func overLimit(r Rule, usage Usage, now time.Time) string {
	_ = now
	if usage == nil {
		return "usage is unknown"
	}
	for _, w := range []struct {
		window time.Duration
		max    int
		name   string
	}{{time.Hour, r.Limits.PerHour, "hour"}, {24 * time.Hour, r.Limits.PerDay, "day"}} {
		if w.max <= 0 {
			return "no limit configured"
		}
		n, err := usage(r.ID, w.window)
		if err != nil {
			return "usage is unknown"
		}
		if n >= w.max {
			return fmt.Sprintf("the limit of %d per %s is used up", w.max, w.name)
		}
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(s)
}
