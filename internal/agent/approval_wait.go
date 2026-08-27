package agent

import (
	"context"
	"errors"
	"sync"
)

type approvalDecision struct {
	Approved bool
	Reason   string
}

type approvalWaiters struct {
	mu sync.Mutex
	ch map[string]chan approvalDecision
}

func newApprovalWaiters() *approvalWaiters {
	return &approvalWaiters{ch: make(map[string]chan approvalDecision)}
}

func (w *approvalWaiters) register(id string) chan approvalDecision {
	if w == nil || id == "" {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if existing, ok := w.ch[id]; ok {
		return existing
	}
	ch := make(chan approvalDecision, 1)
	w.ch[id] = ch
	return ch
}

func (w *approvalWaiters) unregister(id string) {
	if w == nil || id == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.ch, id)
}

func (w *approvalWaiters) notify(id string, decision approvalDecision) bool {
	if w == nil || id == "" {
		return false
	}
	w.mu.Lock()
	ch, ok := w.ch[id]
	if ok {
		delete(w.ch, id)
	}
	w.mu.Unlock()
	if !ok || ch == nil {
		return false
	}
	select {
	case ch <- decision:
		return true
	default:
		return true
	}
}

func waitOnApproval(ctx context.Context, ch <-chan approvalDecision) (approvalDecision, error) {
	if ch == nil {
		if err := ctx.Err(); err != nil {
			return approvalDecision{}, err
		}
		return approvalDecision{}, errors.New("approval waiter is not registered")
	}
	select {
	case decision := <-ch:
		return decision, nil
	case <-ctx.Done():
		return approvalDecision{}, ctx.Err()
	}
}

// NotifyApprovalDecision unblocks a live chat stream waiting on this approval.
// It returns true when a waiter claimed the decision so the caller must not
// execute the tool again or start ResumeApproved.
func (e *Engine) NotifyApprovalDecision(approvalID string, approved bool, reason string) bool {
	if e == nil || e.approvalWaiters == nil {
		return false
	}
	return e.approvalWaiters.notify(approvalID, approvalDecision{Approved: approved, Reason: reason})
}
