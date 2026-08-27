package agent

import (
	"context"
	"log/slog"
	"strings"

	"github.com/actonos/actonos/internal/memory"
)

type conversationContextKey struct{}
type skillCatalogContextKey struct{}

// WithConversationContext scopes semantic retrieval to the active conversation.
func WithConversationContext(ctx context.Context, conversationID string) context.Context {
	if strings.TrimSpace(conversationID) == "" {
		return ctx
	}
	return context.WithValue(ctx, conversationContextKey{}, conversationID)
}

// ConversationIDFromContext returns the active conversation id, if any.
func ConversationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(conversationContextKey{}).(string)
	return strings.TrimSpace(id)
}

// WithSkillCatalog attaches enabled skills so cognitive, planner, and mission
// prompts can inject the same catalog without extra plumbing.
func WithSkillCatalog(ctx context.Context, skills []SkillPromptEntry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(skills) == 0 {
		return ctx
	}
	return context.WithValue(ctx, skillCatalogContextKey{}, skills)
}

// SkillCatalogFrom returns the enabled-skill catalog stored on ctx, if any.
func SkillCatalogFrom(ctx context.Context) []SkillPromptEntry {
	if ctx == nil {
		return nil
	}
	skills, _ := ctx.Value(skillCatalogContextKey{}).([]SkillPromptEntry)
	return skills
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
	extraSkills ...SkillPromptEntry,
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

	if skills := SkillCatalogFrom(ctx); len(skills) > 0 {
		builder.WithSection(&SkillsSection{Skills: skills})
	} else if len(extraSkills) > 0 {
		builder.WithSection(&SkillsSection{Skills: extraSkills})
	}

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
