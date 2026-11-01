package server

import (
	"net/http"
	"strconv"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/memory"
	"github.com/go-chi/chi/v5"
)

// GET /api/agents/{agentID}/insights
func (s *Server) handleListAgentInsights(w http.ResponseWriter, r *http.Request) {
	if s.reflectionEngine == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"proposals": []agent.SelfImprovementProposal{},
			"count":     0,
		})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	status := r.URL.Query().Get("status")

	items, err := s.reflectionEngine.ListProposals(r.Context(), agentID, status)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "INSIGHTS_QUERY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"proposals": items,
		"count":     len(items),
	})
}

// POST /api/agents/{agentID}/insights/{insightID}/apply
func (s *Server) handleApplyAgentInsight(w http.ResponseWriter, r *http.Request) {
	if s.reflectionEngine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "REFLECTION_UNAVAILABLE", "reflection engine is not initialized")
		return
	}

	insightID := chi.URLParam(r, "insightID")
	if err := s.reflectionEngine.ApplyProposal(r.Context(), insightID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "APPLY_INSIGHT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":     "applied",
		"insight_id": insightID,
	})
}

// POST /api/agents/{agentID}/insights/{insightID}/dismiss
func (s *Server) handleDismissAgentInsight(w http.ResponseWriter, r *http.Request) {
	if s.reflectionEngine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "REFLECTION_UNAVAILABLE", "reflection engine is not initialized")
		return
	}

	insightID := chi.URLParam(r, "insightID")
	if err := s.reflectionEngine.DismissProposal(r.Context(), insightID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "DISMISS_INSIGHT_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":     "dismissed",
		"insight_id": insightID,
	})
}

// POST /api/agents/{agentID}/insights/self-review
func (s *Server) handleTriggerSelfReview(w http.ResponseWriter, r *http.Request) {
	if s.reflectionEngine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "REFLECTION_UNAVAILABLE", "reflection engine is not initialized")
		return
	}

	agentID := chi.URLParam(r, "agentID")
	proposals, err := s.reflectionEngine.RunSelfReviewCycle(r.Context(), agentID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "SELF_REVIEW_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":    "self_review_completed",
		"proposals": proposals,
		"count":     len(proposals),
	})
}

// GET /api/agents/{agentID}/memories
func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory engine is not initialized")
		return
	}

	agentID := chi.URLParam(r, "agentID")
	layer := memory.MemoryLayer(r.URL.Query().Get("layer"))
	limit := 50
	if limStr := r.URL.Query().Get("limit"); limStr != "" {
		if l, err := strconv.Atoi(limStr); err == nil && l > 0 {
			limit = l
		}
	}

	mems, err := s.memory.ListMemories(r.Context(), agentID, layer, limit)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "LIST_MEMORIES_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"agent_id": agentID,
		"memories": mems,
		"count":    len(mems),
	})
}

// POST /api/agents/{agentID}/memories/{memoryID}/pin
func (s *Server) handlePinMemory(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory engine is not initialized")
		return
	}

	memoryID := chi.URLParam(r, "memoryID")
	if err := s.memory.PinMemory(r.Context(), memoryID, true); err != nil {
		s.respondError(w, http.StatusInternalServerError, "PIN_MEMORY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":    "pinned",
		"memory_id": memoryID,
	})
}

// DELETE /api/agents/{agentID}/memories/{memoryID}/pin
func (s *Server) handleUnpinMemory(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory engine is not initialized")
		return
	}

	memoryID := chi.URLParam(r, "memoryID")
	if err := s.memory.UnpinMemory(r.Context(), memoryID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "UNPIN_MEMORY_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":    "unpinned",
		"memory_id": memoryID,
	})
}

// PUT /api/agents/{agentID}/memories/{memoryID}/importance
func (s *Server) handleSetMemoryImportance(w http.ResponseWriter, r *http.Request) {
	if s.memory == nil {
		s.respondError(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory engine is not initialized")
		return
	}

	memoryID := chi.URLParam(r, "memoryID")
	var req struct {
		Importance string `json:"importance"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	tier := memory.ImportanceTier(req.Importance)
	if tier == "" {
		tier = memory.ImportanceNormal
	}

	if err := s.memory.SetMemoryImportance(r.Context(), memoryID, tier); err != nil {
		s.respondError(w, http.StatusInternalServerError, "UPDATE_IMPORTANCE_FAILED", err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":     "updated",
		"memory_id":  memoryID,
		"importance": string(tier),
	})
}
