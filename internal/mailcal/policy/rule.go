// Package policy decides whether a mail or calendar write may run without a human.
// It is pure: it takes the exact payload and the active rules and returns a decision.
// Any error, ambiguity or missing fact resolves to needs_approval, never to allow.
package policy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
)

// Action is the kind of write a rule talks about.
type Action string

const (
	ActionReply          Action = "reply"
	ActionSendNew        Action = "send_new"
	ActionCalendarAccept Action = "calendar_accept"
	ActionCalendarCreate Action = "calendar_create"
	ActionCalendarUpdate Action = "calendar_update"
)

// Effect is what a matching rule does.
type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// BodyMode controls how much freedom an allow rule gives the text.
type BodyMode string

const (
	BodyTemplate BodyMode = "template" // body must equal Template exactly
	BodyFree     BodyMode = "free"     // any text; the UI warns this can leak data to the sender
)

// Rule is one entry of the rule book.
type Rule struct {
	ID                string     `json:"id"`
	Enabled           bool       `json:"enabled"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	AccountID         string     `json:"account_id"`
	Action            Action     `json:"action"`
	Effect            Effect     `json:"effect"`
	AllowPublicDomain bool       `json:"allow_public_domain,omitempty"`
	Match             Match      `json:"match"`
	Limits            Limits     `json:"limits"`
}

// Match narrows when a rule applies.
type Match struct {
	Recipients Recipients     `json:"recipients"`
	ReplyOnly  bool           `json:"reply_only,omitempty"`
	Body       BodyMatch      `json:"body"`
	Calendar   *CalendarMatch `json:"calendar,omitempty"`
}

// Recipients lists exact addresses and exact domains (no wildcards, no subdomain matching).
type Recipients struct {
	Addresses []string `json:"addresses,omitempty"`
	Domains   []string `json:"domains,omitempty"`
}

// BodyMatch constrains the message text of an allow rule.
type BodyMatch struct {
	Mode       BodyMode `json:"mode"`
	Template   string   `json:"template,omitempty"`
	AllowLinks bool     `json:"allow_links,omitempty"`
}

// CalendarMatch constrains calendar writes.
type CalendarMatch struct {
	WorkingHours string   `json:"working_hours,omitempty"` // "09:00-18:00" in the event's own time zone
	MaxMinutes   int      `json:"max_minutes,omitempty"`
	Domains      []string `json:"attendee_domains,omitempty"`
}

// Limits caps how often a rule may fire. Zero means "no cap" only for deny rules.
type Limits struct {
	PerHour int `json:"per_hour,omitempty"`
	PerDay  int `json:"per_day,omitempty"`
}

// ErrInvalidRule is returned by Validate.
var ErrInvalidRule = errors.New("invalid rule")

var publicDomains = map[string]struct{}{
	"gmail.com": {}, "googlemail.com": {}, "outlook.com": {}, "hotmail.com": {}, "live.com": {},
	"msn.com": {}, "yahoo.com": {}, "icloud.com": {}, "me.com": {}, "proton.me": {},
	"protonmail.com": {}, "aol.com": {}, "gmx.com": {}, "yandex.com": {}, "qq.com": {},
	"163.com": {}, "mail.com": {}, "zoho.com": {},
}

// IsPublicDomain reports whether d is a mass-market mailbox provider.
func IsPublicDomain(d string) bool {
	_, ok := publicDomains[strings.ToLower(strings.TrimSpace(d))]
	return ok
}

func normalizeDomain(raw string) (string, error) {
	d := strings.TrimSpace(raw)
	if d == "" || strings.ContainsAny(d, "*?@ \t") || strings.HasPrefix(d, ".") || !strings.Contains(d, ".") {
		return "", fmt.Errorf("%w: %q is not a specific domain", ErrInvalidRule, raw)
	}
	e, err := abi.NormalizeEmail("x@" + d)
	if err != nil {
		return "", fmt.Errorf("%w: domain %q: %v", ErrInvalidRule, raw, err)
	}
	return abi.DomainOf(e), nil
}

// Validate normalizes the rule in place and rejects anything too broad to be safe.
func (r *Rule) Validate() error {
	if strings.TrimSpace(r.AccountID) == "" {
		return fmt.Errorf("%w: account is required", ErrInvalidRule)
	}
	switch r.Action {
	case ActionReply, ActionSendNew, ActionCalendarAccept, ActionCalendarCreate, ActionCalendarUpdate:
	default:
		return fmt.Errorf("%w: unknown action %q", ErrInvalidRule, r.Action)
	}
	switch r.Effect {
	case EffectAllow, EffectDeny:
	default:
		return fmt.Errorf("%w: effect must be allow or deny", ErrInvalidRule)
	}
	var err error
	if r.Match.Recipients.Addresses, err = normalizeAddressList(r.Match.Recipients.Addresses); err != nil {
		return err
	}
	if r.Match.Recipients.Domains, err = r.normalizeDomains(r.Match.Recipients.Domains); err != nil {
		return err
	}
	if r.Effect == EffectAllow {
		isMail := r.Action == ActionReply || r.Action == ActionSendNew
		hasWho := len(r.Match.Recipients.Addresses)+len(r.Match.Recipients.Domains) > 0
		if isMail && !hasWho {
			return fmt.Errorf("%w: an allow rule needs at least one address or domain", ErrInvalidRule)
		}
		if r.Action == ActionReply {
			r.Match.ReplyOnly = true // a reply rule can never be used to start new conversations
		}
		if isMail {
			switch r.Match.Body.Mode {
			case BodyTemplate:
				if strings.TrimSpace(r.Match.Body.Template) == "" {
					return fmt.Errorf("%w: template mode needs a template", ErrInvalidRule)
				}
			case BodyFree:
			default:
				return fmt.Errorf("%w: body mode must be template or free", ErrInvalidRule)
			}
		}
		if r.Action == ActionCalendarCreate || r.Action == ActionCalendarUpdate || r.Action == ActionCalendarAccept {
			c := r.Match.Calendar
			if c == nil || len(c.Domains) == 0 {
				return fmt.Errorf("%w: a calendar allow rule needs attendee_domains", ErrInvalidRule)
			}
			if r.Action != ActionCalendarAccept && (c.MaxMinutes <= 0 || c.WorkingHours == "") {
				return fmt.Errorf("%w: creating or updating events needs max_minutes and working_hours", ErrInvalidRule)
			}
		}
		if r.Limits.PerHour <= 0 || r.Limits.PerDay <= 0 {
			return fmt.Errorf("%w: an allow rule needs per_hour and per_day limits", ErrInvalidRule)
		}
	}
	if c := r.Match.Calendar; c != nil {
		if c.Domains, err = r.normalizeDomains(c.Domains); err != nil {
			return err
		}
		if c.WorkingHours != "" {
			if _, _, err := parseHours(c.WorkingHours); err != nil {
				return err
			}
		}
		if c.MaxMinutes < 0 {
			return fmt.Errorf("%w: max_minutes must not be negative", ErrInvalidRule)
		}
	}
	return nil
}

func (r *Rule) normalizeDomains(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		d, err := normalizeDomain(raw)
		if err != nil {
			return nil, err
		}
		if IsPublicDomain(d) && !r.AllowPublicDomain && r.Effect == EffectAllow {
			return nil, fmt.Errorf("%w: %s is a public mailbox provider; set allow_public_domain to use it", ErrInvalidRule, d)
		}
		out = append(out, d)
	}
	return out, nil
}

func normalizeAddressList(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		if strings.Contains(raw, "*") {
			return nil, fmt.Errorf("%w: wildcards are not allowed: %q", ErrInvalidRule, raw)
		}
		e, err := abi.NormalizeEmail(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: address %q: %v", ErrInvalidRule, raw, err)
		}
		out = append(out, e)
	}
	return out, nil
}

func parseHours(s string) (start, end int, err error) {
	var sh, sm, eh, em int
	if _, e := fmt.Sscanf(s, "%d:%d-%d:%d", &sh, &sm, &eh, &em); e != nil {
		return 0, 0, fmt.Errorf("%w: working_hours must look like 09:00-18:00", ErrInvalidRule)
	}
	if sh < 0 || sh > 23 || eh < 0 || eh > 24 || sm < 0 || sm > 59 || em < 0 || em > 59 {
		return 0, 0, fmt.Errorf("%w: working_hours out of range", ErrInvalidRule)
	}
	start, end = sh*60+sm, eh*60+em
	if end <= start {
		return 0, 0, fmt.Errorf("%w: working_hours must end after it starts", ErrInvalidRule)
	}
	return start, end, nil
}
