package mailcal

import (
	"context"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/mailcal/abi"
	"github.com/actonos/actonos/internal/mailcal/policy"
)

const (
	// PhaseBounce marks an audit line for a returned message seen in the inbox.
	PhaseBounce = "bounce"
	// bounceLimit returned messages inside bounceWindow pause every rule of the account.
	bounceLimit  = 3
	bounceWindow = time.Hour
)

// LooksLikeBounce is a deliberately simple heuristic over the sender and subject. A false
// positive only pauses automatic sending, which a person can resume; a false negative
// leaves the other limits in place.
func LooksLikeBounce(m abi.Message) bool {
	from := strings.ToLower(m.From.Email)
	subj := strings.ToLower(m.Subject)
	switch {
	case strings.HasPrefix(from, "mailer-daemon@"), strings.HasPrefix(from, "postmaster@"):
		return true
	case strings.Contains(subj, "undelivered mail returned to sender"),
		strings.Contains(subj, "delivery status notification"),
		strings.Contains(subj, "mail delivery failed"),
		strings.Contains(subj, "undeliverable:"):
		return true
	}
	return false
}

// observeInbox records new bounces and pauses the account's rules at the threshold.
// It is called with the results of reads, so it needs no extra provider traffic.
func (s *Service) observeInbox(ctx context.Context, acct Account, msgs []abi.Message) {
	ctx = context.WithoutCancel(ctx)
	for _, m := range msgs {
		if !LooksLikeBounce(m) {
			continue
		}
		seen, err := s.decisions.HasBounce(ctx, acct.ID, m.ID)
		if err != nil || seen {
			continue
		}
		_ = s.decisions.Record(ctx, DecisionRecord{Phase: PhaseBounce, AccountID: acct.ID, Tool: "inbox", DraftID: m.ID,
			Verdict: policy.VerdictNeedsApproval, Outcome: "returned message observed"})
	}
	n, err := s.decisions.CountSince(ctx, acct.ID, PhaseBounce, bounceWindow)
	if err != nil || n < bounceLimit {
		return
	}
	if paused, err := s.rules.PauseAccount(ctx, acct.ID, "automatic pause: several returned messages in one hour"); err == nil && paused > 0 {
		_ = s.decisions.Record(ctx, DecisionRecord{Phase: PhaseBounce, AccountID: acct.ID, Tool: "auto_pause",
			Verdict: policy.VerdictDeny, Outcome: "rules paused"})
	}
}
