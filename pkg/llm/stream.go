package llm

import "errors"

// ErrStreamingUnsupported is returned by ChatStream when the provider
// does not implement streaming.
var ErrStreamingUnsupported = errors.New("streaming not supported by this provider")

// StreamEventType distinguishes the kind of streaming event.
type StreamEventType string

const (
	StreamEventTextDelta        StreamEventType = "text_delta"
	StreamEventReasoningDelta   StreamEventType = "reasoning_delta"
	StreamEventReasoningApplied StreamEventType = "reasoning_applied"
	StreamEventToolDelta        StreamEventType = "tool_delta"
	StreamEventStop             StreamEventType = "stop"
	StreamEventUsage            StreamEventType = "usage"
)

// PartialResponse represents a single event in a streaming response.
// Its fields are deltas (incremental), not full values.
type PartialResponse struct {
	// Type distinguishes the kind of streaming event.
	Type StreamEventType

	// TextDelta is non-empty when Type == StreamEventTextDelta.
	TextDelta string

	// ReasoningDelta is non-empty when Type == StreamEventReasoningDelta.
	ReasoningDelta string

	// AppliedReasoning is non-nil when Type == StreamEventReasoningApplied.
	AppliedReasoning *AppliedReasoning

	// ToolCallDelta is non-nil when Type == StreamEventToolDelta.
	ToolCallDelta *ToolCallDelta

	// InputTokens is non-zero when Type == StreamEventUsage.
	InputTokens int

	// OutputTokens is non-zero when Type == StreamEventUsage.
	OutputTokens int

	// StopReason is set when Type == StreamEventStop.
	StopReason StopReason
}

// ToolCallDelta carries incremental updates to a tool call.
// Arguments are accumulated by concatenating ArgumentsDelta values
// until the tool call is complete.
type ToolCallDelta struct {
	Index             int    // index of the tool call in the list
	ID                string // populated on first event for this tool
	Name              string // populated when function name arrives
	ArgumentsDelta    string // raw JSON string fragment for arguments
	ArgumentsComplete bool   // true when this tool call's arguments are done
}
