package llm

import (
	"context"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

// Role represents a message sender role.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// StopReason indicates why the LLM stopped generating.
type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonToolUse   StopReason = "tool_use"
	StopReasonMaxTokens StopReason = "max_tokens"
)

// BlockType discriminates the kind of content block.
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
)

// ToolResult holds data for a single tool execution result.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id,omitempty"`
	Content    string `json:"content,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
}

// ContentBlock represents a single content element within a message.
// The Type field discriminates between text, tool_use, and tool_result blocks.
type ContentBlock struct {
	Type BlockType `json:"type"`

	// text block fields
	Text string `json:"text,omitempty"`

	// tool_use block fields (embedded)
	schema.ToolCall

	// tool_result block fields (embedded)
	ToolResult
}

// Message represents a single message in a conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// NewTextMessage creates a Message with a single text content block.
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role:    role,
		Content: []ContentBlock{{Type: BlockText, Text: text}},
	}
}

// NewToolResultMessage creates a tool-role Message containing tool result blocks.
func NewToolResultMessage(results []ToolResult) Message {
	blocks := make([]ContentBlock, len(results))
	for i, r := range results {
		blocks[i] = ContentBlock{
			Type:       BlockToolResult,
			ToolResult: r,
		}
	}
	return Message{Role: RoleTool, Content: blocks}
}

// Response is the unified LLM response.
type Response struct {
	StopReason   StopReason        `json:"stop_reason"`
	Text         string            `json:"text"`
	ToolCalls    []schema.ToolCall `json:"tool_calls,omitempty"`
	InputTokens  int               `json:"input_tokens"`
	OutputTokens int               `json:"output_tokens"`
	Raw          interface{}       `json:"-"`
}

// TotalTokens returns the sum of input and output tokens.
func (r *Response) TotalTokens() int {
	return r.InputTokens + r.OutputTokens
}

// ChatParams holds all per-request parameters for a Chat call.
type ChatParams struct {
	System   string                `json:"system,omitempty"`
	Messages []Message             `json:"messages"`
	Tools    []schema.ToolDefinition `json:"tools,omitempty"`
}

// Provider is the unified interface for LLM providers.
type Provider interface {
	// Chat sends a conversation and returns the LLM response.
	Chat(ctx context.Context, params ChatParams) (*Response, error)

	// Name returns the provider name (for logging/tracing).
	Name() string

	// EstimateTokens provides a rough token count estimate for the given messages.
	EstimateTokens(messages []Message) int
}
