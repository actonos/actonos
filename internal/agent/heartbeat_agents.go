package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/tools"
)

func (h *HeartbeatDaemon) checkCustomAgentPulses(ctx context.Context, now time.Time, manual bool) {
	if h.agentMgr == nil || h.engine == nil || !h.engine.HasConfiguredLLM() {
		return
	}

	agents, err := h.agentMgr.List(ctx)
	if err != nil || len(agents) == 0 {
		return
	}

	for _, a := range agents {
		if a.IsSystem || a.Status != StatusActive || a.HeartbeatConfig == nil || !a.HeartbeatConfig.Enabled {
			continue
		}

		cfg := a.HeartbeatConfig
		if !manual && !withinAgentActiveHours(cfg, now) {
			h.recordSkip(a.AgentID, "active_hours")
			continue
		}
		intervalMin := cfg.IntervalMinutes
		if intervalMin < 5 {
			intervalMin = 5
		}
		h.mu.Lock()
		lastPulse := h.lastAgentPulse[a.AgentID]
		h.mu.Unlock()
		if !manual && !lastPulse.IsZero() && now.Sub(lastPulse) < time.Duration(intervalMin)*time.Minute {
			h.recordSkip(a.AgentID, "interval")
			continue
		}

		directives := strings.TrimSpace(cfg.Directives)
		if !hasActionableHeartbeatDirectives(directives) {
			continue
		}

		routineCtx := context.WithValue(ctx, "suppress_episodic_memory", false)
		routineCtx = context.WithValue(routineCtx, "heartbeat_headless_mode", true)
		routineCtx = WithExecutionSource(routineCtx, "heartbeat")
		if h.cronSched != nil && h.cronSched.CountJobsForAgent(a.AgentID) >= DefaultCronJobsPerAgent {
			routineCtx = tools.WithDeniedTools(routineCtx, "native_cron_schedule")
		}

		prompt := BuildHeartbeatPulsePrompt(directives, fmt.Sprintf("Autonomous pulse for agent: %s (%s)", a.Name, a.AgentID))

		resp, execErr := h.engine.ExecuteStepWithHistory(routineCtx, a.AgentID, prompt, nil)

		run := &HeartbeatRun{
			AgentID:    a.AgentID,
			ExecutedAt: now,
		}
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		run.ID = "hb_" + hex.EncodeToString(b)

		if execErr != nil {
			run.Status = "error"
			run.Summary = execErr.Error()
			slog.Warn("custom agent heartbeat execution failed", "agent_id", a.AgentID, "error", execErr)
		} else if resp != nil {
			run.TokensUsed = resp.Usage.TotalTokens
			trimmed := strings.TrimSpace(resp.Content)

			if h.sessionMgr != nil {
				hbConvID, _ := h.sessionMgr.GetOrCreateSession(ctx, "system", "heartbeat", fmt.Sprintf("Heartbeat: %s", a.Name), "Routine Heartbeat Pulse", a.AgentID)
				_ = h.sessionMgr.SaveMessage(ctx, hbConvID, a.AgentID, "user", fmt.Sprintf("[Heartbeat Pulse]\nStanding Directives: %s", directives), nil)
				_ = h.sessionMgr.SaveMessage(ctx, hbConvID, a.AgentID, "assistant", resp.Content, resp.ToolCalls)
			}

			h.mu.RLock()
			ackMaxChars := h.ackMaxChars
			h.mu.RUnlock()
			isAck, alertText := classifyHeartbeatResponse(trimmed, ackMaxChars)

			if !isAck && len(resp.ToolCalls) == 0 && looksLikeIdleChatter(alertText) {
				isAck = true
			}

			if isAck {
				run.Status = "ok"
				run.Summary = fmt.Sprintf("Agent %s nominal. No proactive notification required.", a.Name)
			} else {
				run.Status = "action_taken"
				run.Summary = shortSummary(alertText, 250)

				alreadyNotified := false
				for _, tc := range resp.ToolCalls {
					if tc.Function.Name == "native_channel_notify" || tc.Function.Name == "channel_notify" {
						alreadyNotified = true
						break
					}
				}

				targetChannel := cfg.TargetChannel
				if targetChannel == "" {
					targetChannel = "all"
				}
				targetAccount := cfg.TargetAccountID

				if h.eventBus != nil && !alreadyNotified && targetChannel != "none" {
					h.eventBus.Publish(bus.NewEvent(bus.EventAgentActionDone, a.AgentID, map[string]any{
						"type":              "proactive_cron_notification",
						"job_name":          fmt.Sprintf("Pulse: %s", a.Name),
						"content":           cleanFullContent(alertText),
						"target_channel":    targetChannel,
						"target_account_id": targetAccount,
						"target_recipient":  "",
					}))
				}
			}
		}

		h.recordRun(*run)
		h.mu.Lock()
		h.lastAgentPulse[a.AgentID] = now
		h.mu.Unlock()
	}
}

func withinAgentActiveHours(cfg *AgentHeartbeatConfig, now time.Time) bool {
	if cfg == nil || (cfg.ActiveHoursStart == "" && cfg.ActiveHoursEnd == "") {
		return true
	}
	startMin, okS := parseHHMM(cfg.ActiveHoursStart)
	endMin, okE := parseHHMM(cfg.ActiveHoursEnd)
	if !okS || !okE {
		return true
	}
	if startMin == endMin {
		return false
	}
	loc := time.UTC
	if cfg.ActiveHoursTimezone != "" {
		if l, err := time.LoadLocation(cfg.ActiveHoursTimezone); err == nil {
			loc = l
		}
	}
	local := now.In(loc)
	nowMin := local.Hour()*60 + local.Minute()
	if startMin < endMin {
		return nowMin >= startMin && nowMin < endMin
	}
	return nowMin >= startMin || nowMin < endMin
}

// heartbeatOKToken is OpenClaw's canonical acknowledgement token: when it
// appears at the very start or end of a routine heartbeat reply (with a
// short enough remainder), the cycle is treated as silent/nominal and
// nothing is delivered to the user.
// See https://docs.openclaw.ai/vi/gateway/heartbeat.
const heartbeatOKToken = "HEARTBEAT_OK"

// defaultAckMaxChars bounds how much leading/trailing commentary may
// accompany HEARTBEAT_OK before a reply is treated as a real alert instead of
// a silent acknowledgement (OpenClaw default: 300).
const defaultAckMaxChars = 300

// parseHHMM parses an "HH:MM" clock time into minutes-since-midnight,
// accepting "24:00" as the end of a day.
func parseHHMM(v string) (int, bool) {
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	hh, errH := strconv.Atoi(parts[0])
	mm, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || hh < 0 || hh > 24 || mm < 0 || mm > 59 || (hh == 24 && mm != 0) {
		return 0, false
	}
	return hh*60 + mm, true
}
