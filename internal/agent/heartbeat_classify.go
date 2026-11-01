package agent

import (
	"regexp"
	"strings"
	"unicode"

)

// classifyHeartbeatResponse implements OpenClaw's heartbeat response
// contract: HEARTBEAT_OK (and standard nominal/OK variations) is treated as an
// acknowledgement when it appears at the boundary of the trimmed reply or matches
// nominal status patterns AND the remaining content is within ackMaxChars. Returns
// (isAck, alertText) — alertText is only meaningful when isAck is false.
func classifyHeartbeatResponse(content string, ackMaxChars int) (bool, string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return true, ""
	}
	if ackMaxChars <= 0 {
		ackMaxChars = defaultAckMaxChars
	}

	nominalTokens := []string{
		heartbeatOKToken,
		"SYSTEM_NOMINAL",
		"ALL_NOMINAL",
		"ZERO_TASKS_PENDING",
	}
	for _, tok := range nominalTokens {
		if rest, ok := stripBoundaryToken(trimmed, tok, true); ok && len(rest) <= ackMaxChars {
			return true, ""
		}
		if rest, ok := stripBoundaryToken(trimmed, tok, false); ok && len(rest) <= ackMaxChars {
			return true, ""
		}
	}

	// Double-barrier safety layer: Check if response is an affirmative/nominal status without new actionable alerts
	lower := strings.ToLower(trimmed)
	cleanLower := strings.Trim(lower, " .!;\n\r\t*`\"'")
	nominalExactPhrases := []string{
		"heartbeat ok",
		"heartbeat_ok",
		"ok",
		"system ok",
		"everything is ok",
		"everything is nominal",
		"system nominal",
		"system is nominal",
		"all systems nominal",
		"all systems operational",
		"zero tasks pending",
		"no actionable tasks",
		"no actionable items",
		"no proactive notification required",
		"không có vấn đề gì",
		"mọi thứ đều ổn",
		"hệ thống bình thường",
		"hệ thống hoạt động bình thường",
		"không cần hành động",
		"không có tác vụ nào cần xử lý",
		"tất cả đều ổn",
	}
	for _, phrase := range nominalExactPhrases {
		if cleanLower == phrase {
			return true, ""
		}
	}

	return false, trimmed
}

// notificationIntentKeywords are common Vietnamese/English phrasings a user
// uses when they expect a task's result to be delivered somewhere (a chat
// channel, Telegram, etc). Used only to decide whether to WARN about a
// TargetChannel=="none" misconfiguration — never to infer or override the
// actual delivery target, which always remains an explicit structured field.
var notificationIntentKeywords = []string{
	"gửi thông báo", "gửi tới", "gửi đến", "báo cho", "thông báo cho", "gửi cho",
	"send to", "send it to", "notify", "send a message", "send this to",
}

// mentionsNotificationIntent reports whether text appears to ask for the
// result to be delivered/sent somewhere.
func mentionsNotificationIntent(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range notificationIntentKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// idleChatterPatterns match common conversational greeting / capability-menu
// / "how can I help" openers that a model sometimes free-associates instead
// of following the heartbeat directive-execution-or-HEARTBEAT_OK contract.
// These are unambiguous signs of an off-topic, non-substantive reply: a real
// alert never opens by greeting the user or asking what they need.
var idleChatterPatterns = []string{
	"chào bieber", "chào anh", "chào bạn", "xin chào",
	"tôi đã sẵn sàng", "tôi đang sẵn sàng", "em đã sẵn sàng", "sẵn sàng hỗ trợ",
	"bạn cần tôi hỗ trợ", "bạn muốn tôi hỗ trợ", "anh cần em hỗ trợ", "cần hỗ trợ gì",
	"how can i help", "how can i assist", "what would you like", "i'm ready to help",
	"i am ready to help", "ready to assist", "what can i do for you",
}

// looksLikeIdleChatter reports whether content is a short conversational
// greeting/self-introduction with no substantive directive execution result.
// Substantive content (e.g. stories, reports, analysis longer than 250 characters)
// is never treated as idle chatter.
func looksLikeIdleChatter(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	// Substantive content is not idle chatter
	if len([]rune(trimmed)) > 250 {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, pat := range idleChatterPatterns {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}

// stripBoundaryToken removes token from the start (prefix=true) or end
// (prefix=false) of s, requiring a word boundary so e.g. "HEARTBEAT_OKAY"
// does not falsely match "HEARTBEAT_OK". Returns the remaining trimmed text
// and whether the token was actually found at that boundary.
func stripBoundaryToken(s, token string, prefix bool) (string, bool) {
	if prefix {
		if !strings.HasPrefix(s, token) {
			return "", false
		}
		if len(s) > len(token) && isTokenBoundaryRune(rune(s[len(token)])) {
			return "", false
		}
		return strings.TrimSpace(s[len(token):]), true
	}
	if !strings.HasSuffix(s, token) {
		return "", false
	}
	if len(s) > len(token) && isTokenBoundaryRune(rune(s[len(s)-len(token)-1])) {
		return "", false
	}
	return strings.TrimSpace(s[:len(s)-len(token)]), true
}

func isTokenBoundaryRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func cleanFullContent(content string) string {
	cleaned := strings.ReplaceAll(content, "[TASK_COMPLETED]", "")
	reProg := regexp.MustCompile(`\[PROGRESS:\s*\d+%?\]`)
	cleaned = reProg.ReplaceAllString(cleaned, "")
	reBlocked := regexp.MustCompile(`\[TASK_BLOCKED:[^\]]*\]`)
	cleaned = reBlocked.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

func shortSummary(content string, maxLen int) string {
	cleaned := cleanFullContent(content)
	if maxLen > 3 && len(cleaned) > maxLen {
		return cleaned[:maxLen-3] + "..."
	}
	return cleaned
}

func hasActionableHeartbeatDirectives(content string) bool {
	if strings.TrimSpace(content) == legacyDefaultHeartbeatDirective {
		return false
	}

	inComment := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		for inComment || strings.HasPrefix(trimmed, "<!--") {
			start := 0
			if inComment {
				start = 0
			}
			if end := strings.Index(trimmed[start:], "-->"); end >= 0 {
				trimmed = strings.TrimSpace(trimmed[start+end+3:])
				inComment = false
				continue
			}
			inComment = true
			trimmed = ""
			break
		}
		if trimmed == "" ||
			regexp.MustCompile(`^#+(\s|$)`).MatchString(trimmed) ||
			regexp.MustCompile(`^[-*+]\s*(\[[\sXx]?\]\s*)?$`).MatchString(trimmed) ||
			regexp.MustCompile("^```[A-Za-z0-9_-]*$").MatchString(trimmed) {
			continue
		}
		return true
	}
	return false
}
