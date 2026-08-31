package agent

import (
	"fmt"
	"strings"

	"github.com/actonos/actonos/internal/llm"
)

// AgentTemplate represents a pre-packaged agent configuration ready to be instantiated.
type AgentTemplate struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Category    string        `json:"category"` // "development", "operations", "productivity", "security", "analysis"
	Description string        `json:"description"`
	Icon        string        `json:"icon"`
	Author      string        `json:"author"`
	Version     string        `json:"version"`
	Tags        []string      `json:"tags"`
	Manifest    AgentManifest `json:"manifest"`
}

// BuiltinTemplates contains the 15+ curated production-ready agent templates for ActonOS.
var BuiltinTemplates = []AgentTemplate{
	{
		ID:          "code_reviewer",
		Name:        "Code Reviewer & Quality Auditor",
		Category:    "development",
		Description: "Performs deep automated code analysis, checks architectural integrity, enforces coding standards, and identifies security vulnerabilities.",
		Icon:        "Code2",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"code", "review", "git", "quality", "ast"},
		Manifest: AgentManifest{
			AgentID:     "agent_code_reviewer",
			Name:        "Code Reviewer & Quality Auditor",
			Description: "Automated peer reviewer ensuring clean architecture, safety, and high test coverage.",
			AvatarIcon:  "Code2",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:    "anthropic/claude-3-7-sonnet-latest",
				FallbackModel:   "openai/gpt-4o",
				ReasoningEffort: "high",
				MaxTokens:       8192,
			},
			SystemInstructions: `You are an elite Staff Software Engineer and Automated Code Reviewer.
Your mission:
1. Review diffs and source files for logic errors, race conditions, edge cases, memory leaks, and performance regressions.
2. Ensure strict adherence to repository coding conventions, interface-driven design, and structured logging.
3. Recommend concrete, idiomatic refactors with unified diffs rather than generic advice.
4. Verify that unit tests cover all edge cases and boundary conditions.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_file_search",
				"native_file_list",
				"native_exec",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   20.0,
				MaxConcurrentRuns:     3,
				MaxTokensPerHour:      100000,
				AllowedWorkspacePaths: []string{"*"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
	{
		ID:          "daily_standup_reporter",
		Name:        "Daily Standup & Progress Reporter",
		Category:    "productivity",
		Description: "Aggregates commit logs, closed missions, and current tasks into a concise daily standup digest delivered to chat channels.",
		Icon:        "CalendarCheck",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"standup", "report", "productivity", "cron"},
		Manifest: AgentManifest{
			AgentID:     "agent_daily_standup",
			Name:        "Daily Standup Reporter",
			Description: "Generates high-signal daily progress summaries across repositories and missions.",
			AvatarIcon:  "CalendarCheck",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "gemini/gemini-2.5-flash",
				FallbackModel: "openai/gpt-4o-mini",
				MaxTokens:     4096,
			},
			SystemInstructions: `You are the Daily Standup and Progress Reporter for ActonOS.
Your duties:
1. Examine recent git activity and completed autonomous tasks in the past 24 hours.
2. Structure the standup report into three sections:
   - ✅ What was accomplished yesterday (concrete outcomes)
   - 🎯 Focus for today (planned missions & open tasks)
   - ⚠️ Blockers & Items needing attention (approvals, failing builds)
3. Maintain crisp, bulleted, professional formatting with zero filler words.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_exec",
				"native_channel_notify",
			},
			ListenChannels: []string{"*"},
			HeartbeatConfig: &AgentHeartbeatConfig{
				Enabled:             true,
				IntervalMinutes:     1440, // 24h
				Directives:          "At 08:30 each morning, aggregate 24h work logs and publish a standup summary to configured channels.",
				ActiveHoursStart:    "08:00",
				ActiveHoursEnd:      "18:00",
				ActiveHoursTimezone: "UTC",
			},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   10.0,
				MaxConcurrentRuns:     1,
				AllowedWorkspacePaths: []string{"*"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
	{
		ID:          "seo_content_monitor",
		Name:        "SEO & Web Health Monitor",
		Category:    "analysis",
		Description: "Continuously tracks website meta tags, sitemaps, broken links, keyword density, and search engine discoverability.",
		Icon:        "Globe",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"seo", "web", "audit", "marketing"},
		Manifest: AgentManifest{
			AgentID:     "agent_seo_monitor",
			Name:        "SEO & Web Health Monitor",
			Description: "Audits URLs, checks sitemaps, and alerts on missing meta tags or indexing anomalies.",
			AvatarIcon:  "Globe",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "gemini/gemini-2.5-flash",
				FallbackModel: "deepseek/deepseek-chat",
				MaxTokens:     4096,
			},
			SystemInstructions: `You are an SEO Technical Auditor.
Your tasks:
1. Fetch web pages and verify title tags, meta descriptions, Open Graph data, canonical URLs, and schema.org JSON-LD.
2. Verify heading hierarchies (H1 -> H2 -> H3) and ensure no missing alt texts on critical media.
3. Check HTTP status codes for links and report broken redirects.
4. Save structured SEO reports into /data/workspace/reports/seo-latest.md.`,
			AuthorizedTools: []string{
				"native_http_fetch",
				"native_web_search",
				"native_file_write",
				"native_file_read",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   15.0,
				MaxConcurrentRuns:     2,
				AllowedWorkspacePaths: []string{"reports/*", "docs/*"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
	{
		ID:          "customer_support_triage",
		Name:        "Customer Support & Inbound Triage",
		Category:    "productivity",
		Description: "Classifies customer inquiries from Telegram, Discord, Zalo, and Slack, answers FAQs accurately, and escalates urgent issues.",
		Icon:        "Headphones",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"support", "triage", "chat", "channels"},
		Manifest: AgentManifest{
			AgentID:     "agent_support_triage",
			Name:        "Customer Support Triage",
			Description: "First-line support agent providing warm, precise, and polite customer resolutions.",
			AvatarIcon:  "Headphones",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "openai/gpt-4o-mini",
				FallbackModel: "gemini/gemini-2.5-flash",
				MaxTokens:     2048,
			},
			SystemInstructions: `You are a Customer Support Specialist for ActonOS.
1. Respond with warmth, clarity, empathy, and professional polish.
2. Consult repository documentation and FAQs using semantic search before responding.
3. If an issue involves billing, sensitive credentials, or infrastructure outages, politely acknowledge the issue and escalate to human operators.
4. Never invent nonexistent features or guarantee SLA times without verification.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_file_search",
				"native_channel_notify",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   25.0,
				MaxConcurrentRuns:     5,
				AllowedWorkspacePaths: []string{"docs/*", "faq/*"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
	{
		ID:          "inbox_zero_assistant",
		Name:        "Inbox Zero & Email Triage",
		Category:    "productivity",
		Description: "Parses unread inbound messages and emails, synthesizes key requests into bullet points, and drafts polite responses.",
		Icon:        "Mail",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"email", "inbox", "triage", "productivity"},
		Manifest: AgentManifest{
			AgentID:     "agent_inbox_zero",
			Name:        "Inbox Zero Assistant",
			Description: "Synthesizes inbound communications and drafts actionable replies.",
			AvatarIcon:  "Mail",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "anthropic/claude-3-7-sonnet-latest",
				FallbackModel: "openai/gpt-4o",
				MaxTokens:     4096,
			},
			SystemInstructions: `You are an Executive Communications Assistant.
1. Summarize lengthy threads into: Sender, Core Request, Urgency (Low/Medium/High), and Action Required.
2. Prepare draft replies matching the user's communication style and tone.
3. Flag high-stakes contracts, security notices, and executive requests for immediate human review.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_file_write",
				"native_channel_notify",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   15.0,
				AllowedWorkspacePaths: []string{"workspace/drafts/*"},
				RequireHumanApproval:  ApprovalMedium,
			},
		},
	},
	{
		ID:          "bug_triager",
		Name:        "Bug Triager & Crash Diagnostic",
		Category:    "development",
		Description: "Inspects stack traces, analyzes error logs, reproduces edge cases in sandbox, and drafts root-cause fix proposals.",
		Icon:        "Bug",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"bug", "debug", "logs", "crash"},
		Manifest: AgentManifest{
			AgentID:     "agent_bug_triager",
			Name:        "Bug Triager & Diagnostic",
			Description: "Automated debugger analyzing stack traces and proposing surgical fixes.",
			AvatarIcon:  "Bug",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:    "anthropic/claude-3-7-sonnet-latest",
				FallbackModel:   "deepseek/deepseek-reasoner",
				ReasoningEffort: "high",
				MaxTokens:       8192,
			},
			SystemInstructions: `You are an expert Systems Debugger and SRE.
1. When presented with a stack trace or error log:
   - Identify the exact package, file, and line number causing the exception.
   - Trace the causal call chain and identify nil pointer dereferences, off-by-one errors, or concurrency race conditions.
   - Propose a minimal, surgical fix with backward-compatibility preserved.
2. Verify your hypothesis using unit test assertions before concluding.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_file_search",
				"native_exec",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   25.0,
				AllowedWorkspacePaths: []string{"*"},
				RequireHumanApproval:  ApprovalMedium,
			},
		},
	},
	{
		ID:          "pr_reviewer_bot",
		Name:        "Pull Request Automated Gatekeeper",
		Category:    "development",
		Description: "Analyzes pull request diffs, checks test coverage delta, validates commit conventions, and posts structured review summaries.",
		Icon:        "GitPullRequest",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"git", "pr", "github", "ci"},
		Manifest: AgentManifest{
			AgentID:     "agent_pr_reviewer",
			Name:        "PR Gatekeeper",
			Description: "Comprehensive PR reviewer enforcing code quality and test invariants.",
			AvatarIcon:  "GitPullRequest",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "openai/gpt-4o",
				FallbackModel: "anthropic/claude-3-7-sonnet-latest",
				MaxTokens:     8192,
			},
			SystemInstructions: `You are an automated CI/CD Pull Request Gatekeeper.
1. Review git diffs against the master branch.
2. Evaluate:
   - Functional correctness and potential edge cases
   - Test coverage (did the PR add tests for newly introduced logic?)
   - Performance implications and database query complexity
   - Security issues (OWASP top 10, unsanitized inputs)
3. Generate a structured PR review markdown comment with Approve / Request Changes / Comment verdict.`,
			AuthorizedTools: []string{
				"native_exec",
				"native_file_read",
				"native_file_search",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   20.0,
				AllowedWorkspacePaths: []string{"*"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
	{
		ID:          "meeting_scheduler",
		Name:        "Calendar & Meeting Orchestrator",
		Category:    "productivity",
		Description: "Coordinates schedules between multiple participants, identifies optimal meeting slots, and sets up reminders.",
		Icon:        "Calendar",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"calendar", "meeting", "scheduling", "reminders"},
		Manifest: AgentManifest{
			AgentID:     "agent_scheduler",
			Name:        "Meeting Orchestrator",
			Description: "Coordinates free slots across timezones and dispatches calendar invites.",
			AvatarIcon:  "Calendar",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "gemini/gemini-2.5-flash",
				FallbackModel: "openai/gpt-4o-mini",
				MaxTokens:     2048,
			},
			SystemInstructions: `You are a Executive Scheduling Assistant.
1. Clarify timezone differences explicitly when coordinating meetings across regions.
2. Propose 3 distinct time slots prioritizing standard business hours (09:00 - 17:00 local).
3. Confirm agenda, duration, and participant list before finalizing events.`,
			AuthorizedTools: []string{
				"native_cron_schedule",
				"native_channel_notify",
				"native_file_read",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   10.0,
				AllowedWorkspacePaths: []string{"*"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
	{
		ID:          "email_drafter",
		Name:        "Executive Communications & Copywriter",
		Category:    "productivity",
		Description: "Drafts high-impact outreach, partner proposals, customer announcements, and release notes in multiple tones.",
		Icon:        "FileText",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"copywriting", "email", "announcements", "editorial"},
		Manifest: AgentManifest{
			AgentID:     "agent_copywriter",
			Name:        "Executive Copywriter",
			Description: "Crafts polished announcements, executive emails, and marketing copy.",
			AvatarIcon:  "FileText",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "anthropic/claude-3-7-sonnet-latest",
				FallbackModel: "openai/gpt-4o",
				MaxTokens:     4096,
			},
			SystemInstructions: `You are an Executive Communications Specialist.
1. Write with exceptional clarity, crisp rhythm, and zero fluff.
2. Adapt seamlessly between persuasive partner proposals, technical release notes, and empathetic customer notices.
3. Always offer two subject line options for emails: one curiosity-driven and one direct/benefit-driven.`,
			AuthorizedTools: []string{
				"native_file_write",
				"native_file_read",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   15.0,
				AllowedWorkspacePaths: []string{"workspace/drafts/*"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
	{
		ID:          "tech_doc_writer",
		Name:        "Technical Documentation Architect",
		Category:    "development",
		Description: "Authors and maintains Diátaxis-compliant technical documentation, architecture guides, and API specifications.",
		Icon:        "BookOpen",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"docs", "diataxis", "markdown", "architecture"},
		Manifest: AgentManifest{
			AgentID:     "agent_doc_writer",
			Name:        "Doc Architect",
			Description: "Generates Diátaxis documentation, API references, and architecture blueprints.",
			AvatarIcon:  "BookOpen",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "anthropic/claude-3-7-sonnet-latest",
				FallbackModel: "openai/gpt-4o",
				MaxTokens:     8192,
			},
			SystemInstructions: `You are a Senior Technical Documentation Architect adhering to the Diátaxis Framework:
- Tutorials: Learning-oriented step-by-step guides for newcomers.
- How-To Guides: Problem-oriented recipes to accomplish specific tasks.
- Reference: Information-oriented, dry, precise descriptions of APIs and CLI tools.
- Explanation: Understanding-oriented background, architectural rationale, and trade-offs.

Ensure strict Markdown formatting, table alignment, and verify symbol links against codebase reality.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_file_write",
				"native_file_edit",
				"native_file_search",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   20.0,
				AllowedWorkspacePaths: []string{"docs/*", "README.md", "CHANGELOG.md"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
	{
		ID:          "data_analyst",
		Name:        "Data Analyst & Metric Synthesizer",
		Category:    "analysis",
		Description: "Processes tabular datasets, computes summary statistics, identifies trends/outliers, and visualizes analytical insights.",
		Icon:        "BarChart3",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"data", "csv", "sql", "analytics", "charts"},
		Manifest: AgentManifest{
			AgentID:     "agent_data_analyst",
			Name:        "Data Analyst",
			Description: "Analyzes metrics, processes CSV/JSON, and computes business intelligence insights.",
			AvatarIcon:  "BarChart3",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:    "openai/gpt-4o",
				FallbackModel:   "anthropic/claude-3-7-sonnet-latest",
				ReasoningEffort: "medium",
				MaxTokens:       8192,
			},
			SystemInstructions: `You are a Principal Data Analyst.
1. When analyzing dataset files (CSV, JSON, SQL dumps):
   - Check schema, missing values, data distributions, and outliers.
   - Compute mean, median, standard deviation, p95/p99 latencies, and growth rates.
   - Formulate clear data-driven hypotheses and highlight actionable business findings.
2. Output clean Markdown tables and ASCII/Mermaid charts summarizing distributions.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_file_write",
				"native_exec",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   25.0,
				AllowedWorkspacePaths: []string{"*"},
				RequireHumanApproval:  ApprovalMedium,
			},
		},
	},
	{
		ID:          "security_audit_monitor",
		Name:        "Security Auditor & Vault Guard",
		Category:    "security",
		Description: "Continuously checks filesystem permissions, scans for exposed credentials/tokens, and validates cryptographic audit chains.",
		Icon:        "ShieldAlert",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"security", "audit", "vault", "compliance"},
		Manifest: AgentManifest{
			AgentID:     "agent_sec_auditor",
			Name:        "Security Auditor",
			Description: "Monitors tamper-evident audit logs, scans for leaked secrets, and enforces principle of least privilege.",
			AvatarIcon:  "ShieldAlert",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "openai/gpt-4o",
				FallbackModel: "anthropic/claude-3-7-sonnet-latest",
				MaxTokens:     4096,
			},
			SystemInstructions: `You are an Autonomous Security Auditor.
Your core imperatives:
1. Scan workspace files and environment variables for plaintext API keys, private certificates, or unauthorized tokens.
2. Verify that all secret reads go through the hardware-bound AES-256-GCM Vault.
3. Validate tamper-evident SHA-256 audit hash chains and alert immediately upon hash mismatch or forbidden shell command executions.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_file_search",
				"native_sysinfo",
				"native_channel_notify",
			},
			ListenChannels: []string{"*"},
			HeartbeatConfig: &AgentHeartbeatConfig{
				Enabled:         true,
				IntervalMinutes: 60,
				Directives:      "Audit system security invariants, check file permissions in /data, and verify audit log integrity.",
			},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   20.0,
				AllowedWorkspacePaths: []string{"*"},
				RequireHumanApproval:  ApprovalHigh,
			},
		},
	},
	{
		ID:          "token_cost_watcher",
		Name:        "LLM Token & Cost Optimizer",
		Category:    "operations",
		Description: "Monitors hourly and daily LLM token consumption, analyzes provider spend efficiency, and recommends cost-reduction strategies.",
		Icon:        "Coins",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"cost", "tokens", "budget", "optimization"},
		Manifest: AgentManifest{
			AgentID:     "agent_cost_watcher",
			Name:        "Token & Cost Watcher",
			Description: "Tracks token burn rate, models cost trends, and optimizes cascade routing.",
			AvatarIcon:  "Coins",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "gemini/gemini-2.5-flash",
				FallbackModel: "openai/gpt-4o-mini",
				MaxTokens:     2048,
			},
			SystemInstructions: `You are an LLM Infrastructure FinOps Specialist.
1. Continuously evaluate token consumption across agents and models.
2. When monthly spend exceeds 80% of budget:
   - Identify top 3 token-consuming agents and heavy tool call loops.
   - Suggest switching simple classification/extraction tasks to lower-cost cascade tiers.
3. Summarize daily spend in USD vs monthly cap.`,
			AuthorizedTools: []string{
				"native_sysinfo",
				"native_channel_notify",
				"native_file_write",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   5.0,
				AllowedWorkspacePaths: []string{"reports/*"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
	{
		ID:          "database_admin",
		Name:        "Database Administrator & Query Tuner",
		Category:    "operations",
		Description: "Inspects SQLite and external database schemas, analyzes index performance, and tracks storage growth.",
		Icon:        "Database",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"database", "sqlite", "sql", "optimization"},
		Manifest: AgentManifest{
			AgentID:     "agent_db_admin",
			Name:        "Database Administrator",
			Description: "Monitors DB storage, runs VACUUM optimization, and audits index utilization.",
			AvatarIcon:  "Database",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:  "openai/gpt-4o",
				FallbackModel: "anthropic/claude-3-7-sonnet-latest",
				MaxTokens:     4096,
			},
			SystemInstructions: `You are a Principal Database Administrator.
1. Monitor table page allocations, fragmentation, and WAL growth in SQLite.
2. Recommend index additions for frequently filtered columns in audit, tasks, and memory tables.
3. Never execute destructive DDL (DROP TABLE, TRUNCATE) without explicit human confirmation.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_sysinfo",
				"native_exec",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   15.0,
				AllowedWorkspacePaths: []string{"*"},
				RequireHumanApproval:  ApprovalHigh,
			},
		},
	},
	{
		ID:          "test_suite_generator",
		Name:        "Automated Test Suite Generator",
		Category:    "development",
		Description: "Reads source code functions and interfaces, creates comprehensive table-driven unit tests, mock providers, and edge case suites.",
		Icon:        "CheckSquare",
		Author:      "ActonOS Core Team",
		Version:     "1.0.0",
		Tags:        []string{"testing", "unit-test", "golang", "coverage", "tdd"},
		Manifest: AgentManifest{
			AgentID:     "agent_test_generator",
			Name:        "Test Suite Generator",
			Description: "Generates rigorous table-driven unit and integration tests with mock fixtures.",
			AvatarIcon:  "CheckSquare",
			Status:      StatusActive,
			ModelConfig: llm.ModelConfig{
				PrimaryModel:    "anthropic/claude-3-7-sonnet-latest",
				FallbackModel:   "openai/gpt-4o",
				ReasoningEffort: "high",
				MaxTokens:       8192,
			},
			SystemInstructions: `You are a Principal Test Automation Architect.
1. For Go code: generate idiomatic, table-driven tests (*_test.go) covering happy paths, boundary conditions, error propagation, and context cancellation.
2. For TypeScript/React code: generate Vitest/Testing-Library component tests verifying user interaction, loading states, and error handling.
3. Use t.Parallel() and temporary directories (t.TempDir()) cleanly.`,
			AuthorizedTools: []string{
				"native_file_read",
				"native_file_write",
				"native_file_edit",
				"native_file_search",
				"native_exec",
			},
			ListenChannels: []string{"*"},
			DelegationScope: DelegationScope{
				MaxMonthlyBudgetUSD:   20.0,
				AllowedWorkspacePaths: []string{"*"},
				RequireHumanApproval:  ApprovalLow,
			},
		},
	},
}

// ListTemplates returns all built-in templates optionally filtered by category or search query.
func ListTemplates(category, query string) []AgentTemplate {
	cat := strings.TrimSpace(strings.ToLower(category))
	q := strings.TrimSpace(strings.ToLower(query))

	if cat == "" && q == "" {
		return BuiltinTemplates
	}

	var results []AgentTemplate
	for _, tmpl := range BuiltinTemplates {
		if cat != "" && cat != "all" && strings.ToLower(tmpl.Category) != cat {
			continue
		}
		if q != "" {
			nameMatch := strings.Contains(strings.ToLower(tmpl.Name), q)
			descMatch := strings.Contains(strings.ToLower(tmpl.Description), q)
			tagMatch := false
			for _, tag := range tmpl.Tags {
				if strings.Contains(strings.ToLower(tag), q) {
					tagMatch = true
					break
				}
			}
			if !nameMatch && !descMatch && !tagMatch {
				continue
			}
		}
		results = append(results, tmpl)
	}
	return results
}

// GetTemplateByID returns a single template by its ID.
func GetTemplateByID(id string) (*AgentTemplate, error) {
	for _, tmpl := range BuiltinTemplates {
		if tmpl.ID == id {
			return &tmpl, nil
		}
	}
	return nil, fmt.Errorf("agent template with ID %q not found", id)
}
