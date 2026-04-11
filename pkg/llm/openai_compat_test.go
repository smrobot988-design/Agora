package llm

import (
	"testing"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

// --- Interface compliance ---

var _ Provider = (*OpenAICompatProvider)(nil)

// --- Unit tests ---

func TestOpenAICompatProviderName(t *testing.T) {
	p := NewOpenAICompatProvider(OpenAICompatConfig{Name: "test-provider"})
	if p.Name() != "test-provider" {
		t.Fatalf("expected 'test-provider', got %s", p.Name())
	}
}

func TestOpenAICompatProviderEstimateTokens(t *testing.T) {
	p := NewOpenAICompatProvider(OpenAICompatConfig{Name: "test"})
	msgs := []Message{NewTextMessage(RoleUser, "Hello world, this is a test.")}
	tokens := p.EstimateTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive token estimate, got %d", tokens)
	}
}

func TestOpenAICompatProviderEstimateTokensEmpty(t *testing.T) {
	p := NewOpenAICompatProvider(OpenAICompatConfig{Name: "test"})
	tokens := p.EstimateTokens([]Message{})
	if tokens != 100 {
		t.Fatalf("expected default 100, got %d", tokens)
	}
}

// --- Conversion function tests (moved from minimax_test.go) ---

func TestConvertMessagesToOpenAI(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "Hello"),
		NewTextMessage(RoleAssistant, "Hi there"),
	}
	result := convertMessagesToGoOpenAI(messages)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %s, %s", result[0].Role, result[1].Role)
	}
	if result[0].Content != "Hello" || result[1].Content != "Hi there" {
		t.Fatalf("unexpected content: %s, %s", result[0].Content, result[1].Content)
	}
}

func TestConvertMessagesToOpenAIWithToolResult(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "What's the weather?"),
		{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{Type: BlockToolUse, ToolCall: schema.ToolCall{ID: "call_123", Name: "get_weather", Input: map[string]interface{}{"location": "Tokyo"}}},
			},
		},
		{
			Role: RoleTool,
			Content: []ContentBlock{
				{Type: BlockToolResult, ToolResult: ToolResult{ToolCallID: "call_123", Content: `{"temp": "20°C"}`}},
			},
		},
	}
	result := convertMessagesToGoOpenAI(messages)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if result[2].Role != "tool" {
		t.Fatalf("expected role 'tool', got %s", result[2].Role)
	}
	if result[2].ToolCallID != "call_123" {
		t.Fatalf("expected tool_call_id 'call_123', got %s", result[2].ToolCallID)
	}
	if result[2].Content != `{"temp": "20°C"}` {
		t.Fatalf("expected tool result content, got %s", result[2].Content)
	}
}

func TestConvertToolsToOpenAI(t *testing.T) {
	tools := []schema.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather",
			InputSchema: schema.PropertySchema{
				Properties: map[string]interface{}{
					"location": map[string]interface{}{
						"type":        "string",
						"description": "City name",
					},
				},
				Required: []string{"location"},
			},
		},
	}
	result := convertToolsToGoOpenAI(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "function" {
		t.Fatalf("expected type 'function', got %s", result[0].Type)
	}
	if result[0].Function.Name != "get_weather" {
		t.Fatalf("expected name 'get_weather', got %s", result[0].Function.Name)
	}
}
