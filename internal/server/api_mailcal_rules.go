package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/actonos/actonos/internal/mailcal"
	"github.com/actonos/actonos/internal/mailcal/policy"
	"github.com/go-chi/chi/v5"
)

// The rule book is changed only through these handlers, which sit behind RequireAuthMiddleware.
// Agents have no route to them: tools never receive the RuleStore, and loopback egress from
// plugins and tools is refused by internal/security.

func (s *Server) respondRuleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mailcal.ErrRuleNotFound):
		s.respondError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, policy.ErrInvalidRule), errors.Is(err, mailcal.ErrProposalDecided):
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	default:
		s.respondMailcalError(w, err)
	}
}

// GET /api/mailcal/settings
func (s *Server) handleMailcalGetSettings(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"enabled": svc.Settings().Enabled(r.Context())})
}

// PUT /api/mailcal/settings
func (s *Server) handleMailcalPutSettings(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err := svc.Settings().SetEnabled(r.Context(), req.Enabled); err != nil {
		s.respondMailcalError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"enabled": req.Enabled})
}

// GET /api/mailcal/rules?account=
func (s *Server) handleMailcalListRules(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	rules, err := svc.Rules().List(r.Context(), r.URL.Query().Get("account"))
	if err != nil {
		s.respondRuleError(w, err)
		return
	}
	if rules == nil {
		rules = []mailcal.StoredRule{}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"rules": rules, "count": len(rules)})
}

// POST /api/mailcal/rules
func (s *Server) handleMailcalCreateRule(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	var rule policy.Rule
	if err := s.decodeJSON(r, &rule); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	stored, err := svc.Rules().Create(r.Context(), rule, "owner")
	if err != nil {
		s.respondRuleError(w, err)
		return
	}
	s.respondJSON(w, http.StatusCreated, stored)
}

// PUT /api/mailcal/rules/{id}/enabled
func (s *Server) handleMailcalSetRuleEnabled(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	reason := ""
	if !req.Enabled {
		reason = "paused by the owner"
	}
	if err := svc.Rules().SetEnabled(r.Context(), chi.URLParam(r, "id"), req.Enabled, reason); err != nil {
		s.respondRuleError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"enabled": req.Enabled})
}

// DELETE /api/mailcal/rules/{id}
func (s *Server) handleMailcalDeleteRule(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	if err := svc.Rules().Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		s.respondRuleError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// POST /api/mailcal/rules/dry-run  {rule, draft_id}
func (s *Server) handleMailcalDryRun(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	var req struct {
		Rule    policy.Rule `json:"rule"`
		DraftID string      `json:"draft_id"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	dec, err := svc.DryRun(r.Context(), req.Rule, req.DraftID)
	if err != nil {
		s.respondRuleError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, dec)
}

// GET /api/mailcal/proposals?status=pending
func (s *Server) handleMailcalListProposals(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	list, err := svc.Proposals().List(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		s.respondRuleError(w, err)
		return
	}
	if list == nil {
		list = []mailcal.Proposal{}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"proposals": list, "count": len(list)})
}

// POST /api/mailcal/proposals/{id}/approve
func (s *Server) handleMailcalApproveProposal(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	rule, err := svc.Proposals().Approve(r.Context(), chi.URLParam(r, "id"), "owner")
	if err != nil {
		s.respondRuleError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, rule)
}

// POST /api/mailcal/proposals/{id}/reject
func (s *Server) handleMailcalRejectProposal(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	if err := svc.Proposals().Reject(r.Context(), chi.URLParam(r, "id")); err != nil {
		s.respondRuleError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// POST /api/mailcal/drafts/{id}/rule-proposal  (the "approve and create a rule" step)
func (s *Server) handleMailcalProposalFromDraft(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	p, err := svc.ProposalFromDraft(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.respondRuleError(w, err)
		return
	}
	s.respondJSON(w, http.StatusCreated, p)
}

// GET /api/mailcal/drafts/{id}/evaluation  (what the approval card shows)
func (s *Server) handleMailcalEvaluateDraft(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	dec, err := svc.EvaluateDraft(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.respondRuleError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, dec)
}

// GET /api/mailcal/decisions?account=&verdict=&phase=&limit=
func (s *Server) handleMailcalListDecisions(w http.ResponseWriter, r *http.Request) {
	svc := s.mailcalOrFail(w)
	if svc == nil {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	recs, err := svc.Decisions().List(r.Context(), mailcal.DecisionFilter{
		AccountID: q.Get("account"), Verdict: q.Get("verdict"), Phase: q.Get("phase"), Limit: limit,
	})
	if err != nil {
		s.respondRuleError(w, err)
		return
	}
	if recs == nil {
		recs = []mailcal.DecisionRecord{}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"decisions": recs, "count": len(recs)})
}
