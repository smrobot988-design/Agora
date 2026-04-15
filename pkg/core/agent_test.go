package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

// mockProvider returns predefined responses in sequence.
type mockProvider struct {
	responses []*llm.Response
	callIndex int
}

func (m *mockProvider) Chat(ctx context.Context, params llm.ChatParams) (*llm.Response, error) {
	if m.callIndex >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses")
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

func (m *mockProvider) Name() string                          { return "mock" }
func (m *mockProvider) EstimateTokens(msgs []llm.Message) int { return 100 }
func (m *mockProvider) ChatStream(ctx context.Context, params llm.ChatParams, cb func(*llm.PartialResponse)) error {
	return llm.ErrStreamingUnsupported
}

type streamMockProvider struct {
	events []*llm.PartialResponse
}

func (m *streamMockProvider) Chat(ctx context.Context, params llm.ChatParams) (*llm.Response, error) {
	return nil, fmt.Errorf("unexpected non-streaming chat")
}

func (m *streamMockProvider) Name() string { return "stream-mock" }

func (m *streamMockProvider) EstimateTokens(msgs []llm.Message) int { return 100 }

func (m *streamMockProvider) ChatStream(ctx context.Context, params llm.ChatParams, cb func(*llm.PartialResponse)) error {
	for _, event := range m.events {
		cb(event)
	}
	cb(nil)
	return nil
}

// newTestAgent creates an Agent with mock provider, a simple echo tool, and the given responses.
func newTestAgent(t *testing.T, responses []*llm.Response, opts ...AgentOption) *Agent {
	t.Helper()
	registry := tool.NewRegistry()
	_ = registry.Register(&tool.Func{
		Def: schema.ToolDefinition{
			Name:        "echo",
			Description: "Echo the message",
			InputSchema: schema.PropertySchema{
				Properties: map[string]interface{}{
					"message": map[string]interface{}{"type": "string"},
				},
				Required: []string{"message"},
			},
		},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			msg, _ := input["message"].(string)
			return msg, nil
		},
	})

	provider := &mockProvider{responses: responses}
	mem := NewMemory(WithSystemPrompt("test system prompt"))
	return NewAgent(provider, mem, registry, opts...)
}

func TestRunSimpleResponse(t *testing.T) {
	agent := newTestAgent(t, []*llm.Response{
		{StopReason: llm.StopReasonEndTurn, Text: "Hello!", InputTokens: 10, OutputTokens: 5},
	})

	result, err := agent.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello!" {
		t.Fatalf("expected 'Hello!', got %q", result.Text)
	}
	if result.Turns != 1 {
		t.Fatalf("expected 1 turn, got %d", result.Turns)
	}
}

func TestRunWithToolUse(t *testing.T) {
	agent := newTestAgent(t, []*llm.Response{
		{
			StopReason: llm.StopReasonToolUse,
			ToolCalls: []schema.ToolCall{
				{ID: "c1", Name: "echo", Input: map[string]interface{}{"message": "world"}},
			},
			InputTokens: 20, OutputTokens: 10,
		},
		{
			StopReason:  llm.StopReasonEndTurn,
			Text:        "The echo returned: world",
			InputTokens: 30, OutputTokens: 15,
		},
	})

	result, err := agent.Run(context.Background(), "Echo world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "The echo returned: world" {
		t.Fatalf("expected final text, got %q", result.Text)
	}
	if result.Turns != 2 {
		t.Fatalf("expected 2 turns, got %d", result.Turns)
	}
}

func TestRunToolExecutionError(t *testing.T) {
	registry := tool.NewRegistry()
	_ = registry.Register(&tool.Func{
		Def: schema.ToolDefinition{Name: "fail_tool", Description: "Always fails"},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			return "", fmt.Errorf("tool broke")
		},
	})

	provider := &mockProvider{responses: []*llm.Response{
		{
			StopReason:  llm.StopReasonToolUse,
			ToolCalls:   []schema.ToolCall{{ID: "c1", Name: "fail_tool"}},
			InputTokens: 10, OutputTokens: 5,
		},
		{
			StopReason:  llm.StopReasonEndTurn,
			Text:        "The tool failed, sorry.",
			InputTokens: 20, OutputTokens: 10,
		},
	}}

	mem := NewMemory()
	agent := NewAgent(provider, mem, registry)
	result, err := agent.Run(context.Background(), "Try failing tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "The tool failed, sorry." {
		t.Fatalf("expected recovery text, got %q", result.Text)
	}

	// Verify error tool_result was added to memory
	msgs, _ := mem.AllMessages()
	// Messages: user, assistant(tool_use), tool(error result), assistant(final)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	toolMsg := msgs[2]
	if toolMsg.Role != llm.RoleTool {
		t.Fatalf("expected tool role, got %s", toolMsg.Role)
	}
	if !toolMsg.Content[0].IsError {
		t.Fatal("expected tool result to be marked as error")
	}
}

func TestRunMaxTurnsExceeded(t *testing.T) {
	// All responses are tool_use → infinite loop
	responses := make([]*llm.Response, 5)
	for i := range responses {
		responses[i] = &llm.Response{
			StopReason:  llm.StopReasonToolUse,
			ToolCalls:   []schema.ToolCall{{ID: fmt.Sprintf("c%d", i), Name: "echo", Input: map[string]interface{}{"message": "loop"}}},
			InputTokens: 10, OutputTokens: 5,
		}
	}
	agent := newTestAgent(t, responses, WithMaxTurns(3))

	_, err := agent.Run(context.Background(), "Loop forever")
	if err == nil {
		t.Fatal("expected error for max turns exceeded")
	}
}

func TestRunContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	agent := newTestAgent(t, []*llm.Response{
		{StopReason: llm.StopReasonEndTurn, Text: "should not reach"},
	})

	_, err := agent.Run(ctx, "Hi")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestRunTokenAccumulation(t *testing.T) {
	agent := newTestAgent(t, []*llm.Response{
		{
			StopReason:  llm.StopReasonToolUse,
			ToolCalls:   []schema.ToolCall{{ID: "c1", Name: "echo", Input: map[string]interface{}{"message": "a"}}},
			InputTokens: 100, OutputTokens: 50,
		},
		{
			StopReason:  llm.StopReasonEndTurn,
			Text:        "Done",
			InputTokens: 200, OutputTokens: 80,
		},
	})

	result, err := agent.Run(context.Background(), "Test tokens")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalInputTokens != 300 {
		t.Fatalf("expected 300 input tokens, got %d", result.TotalInputTokens)
	}
	if result.TotalOutputTokens != 130 {
		t.Fatalf("expected 130 output tokens, got %d", result.TotalOutputTokens)
	}
	if result.TotalTokens() != 430 {
		t.Fatalf("expected 430 total tokens, got %d", result.TotalTokens())
	}
}

func TestRunMultipleToolCalls(t *testing.T) {
	registry := tool.NewRegistry()
	callCount := 0
	_ = registry.Register(&tool.Func{
		Def: schema.ToolDefinition{Name: "counter", Description: "Counts calls"},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			callCount++
			return fmt.Sprintf("call_%d", callCount), nil
		},
	})

	provider := &mockProvider{responses: []*llm.Response{
		{
			StopReason: llm.StopReasonToolUse,
			ToolCalls: []schema.ToolCall{
				{ID: "c1", Name: "counter"},
				{ID: "c2", Name: "counter"},
			},
			InputTokens: 10, OutputTokens: 5,
		},
		{
			StopReason:  llm.StopReasonEndTurn,
			Text:        "Both calls done",
			InputTokens: 20, OutputTokens: 10,
		},
	}}

	mem := NewMemory()
	agent := NewAgent(provider, mem, registry)
	result, err := agent.Run(context.Background(), "Call twice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Both calls done" {
		t.Fatalf("expected 'Both calls done', got %q", result.Text)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 tool calls, got %d", callCount)
	}
}

func TestRunStreamingReasoningResult(t *testing.T) {
	provider := &streamMockProvider{events: []*llm.PartialResponse{
		{Type: llm.StreamEventReasoningDelta, ReasoningDelta: "thinking"},
		{Type: llm.StreamEventTextDelta, TextDelta: "final"},
		{Type: llm.StreamEventUsage, InputTokens: 10, OutputTokens: 5},
		{Type: llm.StreamEventStop, StopReason: llm.StopReasonEndTurn},
	}}
	mem := NewMemory()
	registry := tool.NewRegistry()
	var eventTypes []llm.StreamEventType

	agent := NewAgent(provider, mem, registry, WithStreamCallback(func(event *llm.PartialResponse) {
		if event != nil {
			eventTypes = append(eventTypes, event.Type)
		}
	}))

	result, err := agent.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "final" {
		t.Fatalf("expected final text, got %q", result.Text)
	}
	if result.ReasoningText != "thinking" {
		t.Fatalf("expected reasoning text, got %q", result.ReasoningText)
	}
	if result.TotalInputTokens != 10 || result.TotalOutputTokens != 5 {
		t.Fatalf("unexpected token totals: %d in / %d out", result.TotalInputTokens, result.TotalOutputTokens)
	}
	if len(eventTypes) < 2 || eventTypes[0] != llm.StreamEventReasoningDelta || eventTypes[1] != llm.StreamEventTextDelta {
		t.Fatalf("unexpected event types: %v", eventTypes)
	}

	msgs, err := mem.AllMessages()
	if err != nil {
		t.Fatalf("unexpected memory error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected user and assistant messages, got %d", len(msgs))
	}
	if got := msgs[1].Content[0].Text; got != "final" {
		t.Fatalf("expected memory to store final text only, got %q", got)
	}
}

func TestNewAgentDefaults(t *testing.T) {
	provider := &mockProvider{}
	mem := NewMemory()
	registry := tool.NewRegistry()
	agent := NewAgent(provider, mem, registry)

	if agent.maxTurns != defaultMaxTurns {
		t.Fatalf("expected default maxTurns %d, got %d", defaultMaxTurns, agent.maxTurns)
	}
}

func TestNewAgentWithOptions(t *testing.T) {
	provider := &mockProvider{}
	mem := NewMemory()
	registry := tool.NewRegistry()

	agent := NewAgent(provider, mem, registry, WithMaxTurns(20))

	if agent.maxTurns != 20 {
		t.Fatalf("expected maxTurns 20, got %d", agent.maxTurns)
	}
}
