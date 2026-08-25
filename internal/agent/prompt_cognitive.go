package agent

import (
	"context"
	"log/slog"

	"github.com/actonos/actonos/internal/memory"
)

type conversationContextKey struct{}

// WithConversationContext scopes semantic retrieval to the active conversation.
func WithConversationContext(ctx context.Context, conversationID string) context.Context {
	return context.WithValue(ctx, conversationContextKey{}, conversationID)
}

// BuildCognitiveSystemPrompt constructs a complete 7-layer Cognitive System Prompt
// using the unified PromptBuilder.
func BuildCognitiveSystemPrompt(
	ctx context.Context,
	agentID string,
	agent *AgentManifest,
	dataDir string,
	workspaceDir string,
	profileMgr *UserProfileManager,
	mem *memory.HybridEngine,
	embedding *memory.EmbeddingService,
	userMessage string,
) (string, int) {
	builder := NewPromptBuilder()

	agentSlug := agentID
	if agent != nil && agent.AgentID != "" {
		agentSlug = agent.AgentID
	}
	if agentSlug == "" {
		agentSlug = DefaultSystemAgentID
	}

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
		DataDir:      dataDir,
		WorkspaceDir: workspaceDir,
		AgentSlug:    agentSlug,
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
	builder.WithSection(&ConstraintsSection{
		DataDir:      dataDir,
		AgentSlug:    agentSlug,
		WorkspaceDir: workspaceDir,
	})

	// Layer 7b: Headless Autonomous Mode Check
	if headless, _ := ctx.Value("heartbeat_headless_mode").(bool); headless {
		builder.WithSection(&HeadlessSection{Active: true})
	}

	// Layer 7: Episodic & Semantic Vector Memory Retrieval
	episodicCount := 0
	suppressEpisodic, _ := ctx.Value("suppress_episodic_memory").(bool)
	if mem != nil && !suppressEpisodic && userMessage != "" {
		var queryVector []float32
		if embedding != nil {
			if vec, err := embedding.EmbedQueryVector(ctx, userMessage); err == nil {
				queryVector = vec
			}
		}
		memories, err := mem.Search(ctx, agentID, memory.LayerEpisodic, userMessage, queryVector, 4)
		if err != nil {
			slog.Debug("episodic memory search error", "agent_id", agentID, "error", err)
		} else if len(memories) > 0 {
			episodicCount = len(memories)
			builder.WithSection(&EpisodicSection{
				Memories: memories,
			})
		}
	}

	semanticCount := 0
	if embedding != nil && !suppressEpisodic && userMessage != "" {
		scopes := []string{"agent:" + agentID, "shared"}
		if conversationID, _ := ctx.Value(conversationContextKey{}).(string); conversationID != "" {
			scopes = append([]string{"conversation:" + conversationID}, scopes...)
		}
		records, err := embedding.Search(ctx, userMessage, scopes, 6)
		if err != nil {
			slog.Warn("semantic knowledge vector search error", "agent_id", agentID, "error", err)
		} else if len(records) > 0 {
			semanticCount = len(records)
			builder.WithSection(&SemanticKnowledgeSection{Records: records})
		}
	}

	totalRetrieved := episodicCount + semanticCount
	if totalRetrieved > 0 {
		slog.Info("layer 7 cognitive memory retrieval completed",
			"agent_id", agentID,
			"episodic_count", episodicCount,
			"semantic_count", semanticCount,
			"total_fragments", totalRetrieved,
		)
	}

	return builder.Build(), totalRetrieved
}
