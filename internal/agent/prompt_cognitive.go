package agent

import (
	"context"

	"github.com/actonos/actonos/internal/memory"
)

// BuildCognitiveSystemPrompt constructs a complete 7-layer Cognitive System Prompt
// using the unified PromptBuilder.
func BuildCognitiveSystemPrompt(
	ctx context.Context,
	agentID string,
	agent *AgentManifest,
	workspaceDir string,
	profileMgr *UserProfileManager,
	mem *memory.HybridEngine,
	userMessage string,
) (string, int) {
	builder := NewPromptBuilder()

	// Layer 0: Universal Demeanor & Directness Standards
	builder.WithSection(&MetaDirectiveSection{})

	// Layer 1: Agent Identity & Manifest
	if agent != nil {
		builder.WithSection(&IdentitySection{
			AgentID:          agent.AgentID,
			AgentName:        agent.Name,
			Description:      agent.Description,
			AuthorizedTools:  agent.AuthorizedTools,
			RoleInstructions: agent.SystemInstructions,
		})
	}

	// Layer 2: Hardware & Environment Grounding
	builder.WithSection(&EnvironmentSection{
		WorkspaceDir: workspaceDir,
	})

	// Layer 3: Agent Soul (if available)
	if profileMgr != nil {
		soul := profileMgr.GetAgentSoul(agentID)
		if soul != "" {
			builder.WithSection(&SoulSection{
				SoulContent: soul,
			})
		}

		// Layer 4: Collaborator Profile
		profile := profileMgr.GetProfile()
		builder.WithSection(&CollaboratorSection{
			Profile: profile,
		})

		// Layer 5: Procedural Knowledge
		patterns, _ := profileMgr.GetRelevantPatterns(ctx, "general")
		if len(patterns) > 0 {
			builder.WithSection(&ProceduralSection{
				Patterns: patterns,
			})
		}
	}

	// Layer 6: Operational & Safety Constraints
	builder.WithSection(&ConstraintsSection{})

	// Layer 7b: Headless Autonomous Mode Check
	if headless, _ := ctx.Value("heartbeat_headless_mode").(bool); headless {
		builder.WithSection(&HeadlessSection{Active: true})
	}

	// Layer 7: Episodic Memory Retrieval
	episodicCount := 0
	suppressEpisodic, _ := ctx.Value("suppress_episodic_memory").(bool)
	if mem != nil && !suppressEpisodic && userMessage != "" {
		memories, err := mem.Search(ctx, agentID, memory.LayerEpisodic, userMessage, nil, 4)
		if err == nil && len(memories) > 0 {
			episodicCount = len(memories)
			builder.WithSection(&EpisodicSection{
				Memories: memories,
			})
		}
	}

	return builder.Build(), episodicCount
}
