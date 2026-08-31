package llm

import "encoding/json"

const DefaultReasoningEffort = "medium"

// Role defines the participant role in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in an LLM conversation.
type Message struct {
	Role             Role       `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	Name             string     `json:"name,omitempty"`
	// ProviderItems carries opaque provider output items needed for a follow-up
	// request (for example Responses reasoning items accompanying tool calls).
	ProviderItems []json.RawMessage `json:"provider_items,omitempty"`
}

// ToolCall represents a structured function call invoked by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall describes the function name and arguments.
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDefinition defines a callable tool schema provided to the LLM.
type ToolDefinition struct {
	Type     string             `json:"type"` // "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition describes tool metadata and JSON Schema parameters.
type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ModelConfig holds model parameters per agent.
type ModelConfig struct {
	PrimaryModel    string  `json:"primary_model"`
	FallbackModel   string  `json:"fallback_model,omitempty"`
	ReasoningEffort string  `json:"reasoning_effort,omitempty"`
	MaxTokens       int     `json:"max_tokens,omitempty"`
	TopP            float64 `json:"top_p,omitempty"`
}

// TaskKind indicates the domain/complexity of an LLM request for smart cascade routing.
type TaskKind string

const (
	TaskKindReasoning TaskKind = "reasoning"
	TaskKindCoding    TaskKind = "coding"
	TaskKindSummarize TaskKind = "summarize"
	TaskKindClassify  TaskKind = "classify"
	TaskKindExtract   TaskKind = "extract"
	TaskKindGeneral   TaskKind = "general"
)

// CompletionOptions configures a completion request.
type CompletionOptions struct {
	Model           string           `json:"model,omitempty"`
	ReasoningEffort string           `json:"reasoning_effort,omitempty"`
	TaskKind        TaskKind         `json:"task_kind,omitempty"`
	MaxTokens       *int             `json:"max_tokens,omitempty"`
	Tools           []ToolDefinition `json:"tools,omitempty"`
	StopWords       []string         `json:"stop_words,omitempty"`
}

// WithDefaults returns options with provider-neutral defaults applied.
func (o CompletionOptions) WithDefaults() CompletionOptions {
	if o.ReasoningEffort == "" {
		o.ReasoningEffort = DefaultReasoningEffort
	}
	return o
}

// EffectiveReasoningEffort returns the configured effort or the platform default.
func (c ModelConfig) EffectiveReasoningEffort() string {
	if c.ReasoningEffort == "" {
		return DefaultReasoningEffort
	}
	return c.ReasoningEffort
}

// Usage captures token metrics from an LLM call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Response represents a non-streaming completion result.
type Response struct {
	Content          string            `json:"content"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
	Usage            Usage             `json:"usage"`
	Model            string            `json:"model"`
	ProviderItems    []json.RawMessage `json:"provider_items,omitempty"`
}

// StreamChunk represents a single chunk during streaming completions.
type StreamChunk struct {
	DeltaContent   string            `json:"delta_content,omitempty"`
	DeltaReasoning string            `json:"delta_reasoning,omitempty"`
	ToolCalls      []ToolCall        `json:"tool_calls,omitempty"`
	Usage          *Usage            `json:"usage,omitempty"`
	Done           bool              `json:"done"`
	Error          error             `json:"error,omitempty"`
	ProviderItems  []json.RawMessage `json:"provider_items,omitempty"`
}
