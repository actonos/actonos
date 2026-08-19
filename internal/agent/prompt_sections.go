package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/memory"
)

// -----------------------------------------------------------------------------
// Layer 0: Meta Directive & Demeanor Section
// -----------------------------------------------------------------------------

type MetaDirectiveSection struct {
	Language string
}

func (s *MetaDirectiveSection) Name() string { return "operating_standards" }
func (s *MetaDirectiveSection) IsEmpty() bool { return false }
func (s *MetaDirectiveSection) Render() string {
	var sb strings.Builder
	sb.WriteString("<operating_standards>\n")
	sb.WriteString("  <rule id=\"language_match\">Always respond in the EXACT language used by the collaborator in their prompt (e.g. Vietnamese if prompted in Vietnamese, English if prompted in English).</rule>\n")
	sb.WriteString("  <rule id=\"direct_delivery\">Deliver concrete answers, summaries, code, and findings directly. NEVER output canned greetings, capability menus, or self-introductions when responding to commands or questions.</rule>\n")
	sb.WriteString("  <rule id=\"zero_robot_cliches\">NEVER produce robotic disclaimers ('As an AI...', 'I do not have feelings...'), generic filler, or repetitive apologies. Embody decisive, competent, authentic partnership.</rule>\n")
	sb.WriteString("  <rule id=\"markdown_clarity\">Format all responses with clean GitHub-flavored Markdown, clear headings, bullet points, and syntax-highlighted code blocks.</rule>\n")
	sb.WriteString("</operating_standards>")
	return sb.String()
}

// -----------------------------------------------------------------------------
// Layer 1: Agent Identity Section
// -----------------------------------------------------------------------------

type IdentitySection struct {
	AgentID          string
	AgentName        string
	Description      string
	AuthorizedTools  []string
	RoleInstructions string
}

func (s *IdentitySection) Name() string { return "agent_identity" }
func (s *IdentitySection) IsEmpty() bool {
	return s.AgentName == "" && s.Description == "" && s.RoleInstructions == ""
}
func (s *IdentitySection) Render() string {
	attrs := map[string]string{
		"id":   s.AgentID,
		"name": s.AgentName,
	}
	var sb strings.Builder
	if s.Description != "" {
		fmt.Fprintf(&sb, "<role_scope>%s</role_scope>\n", s.Description)
	}
	if len(s.AuthorizedTools) > 0 {
		fmt.Fprintf(&sb, "<authorized_tools count=\"%d\">%s</authorized_tools>\n",
			len(s.AuthorizedTools), strings.Join(s.AuthorizedTools, ", "))
	}
	if s.RoleInstructions != "" {
		fmt.Fprintf(&sb, "<specialized_instructions>\n%s\n</specialized_instructions>",
			indentContent(strings.TrimSpace(s.RoleInstructions), "  "))
	}
	return XMLTag("agent_identity", attrs, sb.String())
}

// -----------------------------------------------------------------------------
// Layer 2: Hardware & Environment Grounding Section
// -----------------------------------------------------------------------------

type EnvironmentSection struct {
	WorkspaceDir string
}

func (s *EnvironmentSection) Name() string { return "environment" }
func (s *EnvironmentSection) IsEmpty() bool { return false }
func (s *EnvironmentSection) Render() string {
	return strings.TrimSpace(BuildHostEnvironmentPrompt(s.WorkspaceDir))
}

// -----------------------------------------------------------------------------
// Layer 3: Agent Soul & Temperament Section
// -----------------------------------------------------------------------------

type SoulSection struct {
	SoulContent string
}

func (s *SoulSection) Name() string { return "core_soul" }
func (s *SoulSection) IsEmpty() bool { return strings.TrimSpace(s.SoulContent) == "" }
func (s *SoulSection) Render() string {
	return SimpleTag("core_soul", s.SoulContent)
}

// -----------------------------------------------------------------------------
// Layer 4: Collaborator Profile Section
// -----------------------------------------------------------------------------

type CollaboratorSection struct {
	Profile UserProfile
}

func (s *CollaboratorSection) Name() string { return "collaborator_profile" }
func (s *CollaboratorSection) IsEmpty() bool {
	p := s.Profile
	return p.UserName == "" && p.UserRole == "" && p.Language == "" &&
		p.CommunicationStyle == "" && p.CustomInstructions == "" && len(p.Preferences) == 0
}
func (s *CollaboratorSection) Render() string {
	p := s.Profile
	var sb strings.Builder
	if p.UserName != "" {
		fmt.Fprintf(&sb, "- User: %s\n", p.UserName)
	}
	if p.UserRole != "" {
		fmt.Fprintf(&sb, "- Role: %s\n", p.UserRole)
	}
	userLang := p.Language
	if prefLang, ok := p.Preferences["language"]; ok && prefLang != "" {
		userLang = prefLang
	}
	if userLang != "" {
		fmt.Fprintf(&sb, "- Preferred Language: %s\n", userLang)
	}
	if p.Timezone != "" {
		fmt.Fprintf(&sb, "- Timezone: %s (Current Time: %s UTC)\n", p.Timezone, time.Now().UTC().Format(time.RFC3339))
	}
	if p.CommunicationStyle != "" {
		fmt.Fprintf(&sb, "- Communication Style: %s\n", p.CommunicationStyle)
	}
	if p.CustomInstructions != "" {
		fmt.Fprintf(&sb, "- Standing Directives: %s\n", p.CustomInstructions)
	}
	for k, v := range p.Preferences {
		if k != "language" && v != "" {
			fmt.Fprintf(&sb, "- Preference [%s]: %s\n", k, v)
		}
	}
	return SimpleTag("collaborator_profile", sb.String())
}

// -----------------------------------------------------------------------------
// Layer 5: Procedural Knowledge Section
// -----------------------------------------------------------------------------

type ProceduralSection struct {
	Patterns []ProceduralPattern
}

func (s *ProceduralSection) Name() string { return "procedural_knowledge" }
func (s *ProceduralSection) IsEmpty() bool { return len(s.Patterns) == 0 }
func (s *ProceduralSection) Render() string {
	var sb strings.Builder
	for _, p := range s.Patterns {
		attrs := map[string]string{
			"name":   p.PatternName,
			"domain": p.Domain,
		}
		sb.WriteString(XMLTag("workflow", attrs, p.Workflow))
		sb.WriteString("\n")
	}
	return SimpleTag("procedural_knowledge", sb.String())
}

// -----------------------------------------------------------------------------
// Layer 6: Operational & Safety Constraints Section
// -----------------------------------------------------------------------------

type ConstraintsSection struct {
	AdditionalRules []string
}

func (s *ConstraintsSection) Name() string { return "operational_constraints" }
func (s *ConstraintsSection) IsEmpty() bool { return false }
func (s *ConstraintsSection) Render() string {
	var sb strings.Builder
	sb.WriteString("<operational_constraints>\n")
	sb.WriteString("  <rule id=\"no_interactive_commands\">NEVER execute interactive blocking commands (e.g. `vim`, `nano`, `vi`, `top`, `htop`, `less` without `-F`, `more`, or interactive `python` REPL without `-c`). They will hang subshell execution.</rule>\n")
	sb.WriteString("  <rule id=\"limit_output_volume\">When inspecting files or running commands, ALWAYS limit output volume (e.g. `head -n 50`, `tail -n 50`, `rg -m 20`, `jq '.[:5]'`). Do NOT cat multi-megabyte files into the context window.</rule>\n")
	sb.WriteString("  <rule id=\"tool_domain_relevance\">Only invoke tools directly related to the user request. For web searches, use `native_web_search`; do NOT randomly inspect workspace files or hardware telemetry unless requested.</rule>\n")
	sb.WriteString("  <rule id=\"verify_modifications\">After modifying code, files, or executing system changes, verify the status code and observations before declaring task completion.</rule>\n")
	for _, r := range s.AdditionalRules {
		if r != "" {
			fmt.Fprintf(&sb, "  <rule>%s</rule>\n", r)
		}
	}
	sb.WriteString("</operational_constraints>")
	return sb.String()
}

// -----------------------------------------------------------------------------
// Layer 7: Episodic Memory Section
// -----------------------------------------------------------------------------

type EpisodicSection struct {
	Memories []memory.MemoryRecord
}

func (s *EpisodicSection) Name() string { return "episodic_memories" }
func (s *EpisodicSection) IsEmpty() bool { return len(s.Memories) == 0 }
func (s *EpisodicSection) Render() string {
	var sb strings.Builder
	for _, m := range s.Memories {
		fmt.Fprintf(&sb, "- %s\n", strings.TrimSpace(m.Content))
	}
	return SimpleTag("episodic_memories", sb.String())
}

// -----------------------------------------------------------------------------
// Layer 7b: Headless Autonomous Execution Mode Section
// -----------------------------------------------------------------------------

type HeadlessSection struct {
	Active bool
}

func (s *HeadlessSection) Name() string { return "autonomous_headless_mode" }
func (s *HeadlessSection) IsEmpty() bool { return !s.Active }
func (s *HeadlessSection) Render() string {
	var sb strings.Builder
	sb.WriteString("<autonomous_headless_mode>\n")
	sb.WriteString("  <directive>This is an unattended background automation cycle. NO human is reading this in real time.</directive>\n")
	sb.WriteString("  <directive>NEVER reply with a greeting, self-introduction, or a question like 'how can I help?'.</directive>\n")
	sb.WriteString("  <directive>You MUST execute the standing directive using authorized tools and report the concrete result, OR reply with EXACTLY `HEARTBEAT_OK` if there is nothing actionable.</directive>\n")
	sb.WriteString("</autonomous_headless_mode>")
	return sb.String()
}
