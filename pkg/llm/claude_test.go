package llm

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

var testTools = []schema.ToolDefinition{
	{
		Name:        "get_weather",
		Description: "Get the current weather for a location",
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

func TestConvertMessagesToSDK(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "Hello"),
		NewTextMessage(RoleAssistant, "Hi there"),
	}
	result, err := convertMessagesToSDK(messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
}

func TestConvertMessagesWithToolResult(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "What's the weather?"),
		{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{Type: BlockToolUse, ToolCall: schema.ToolCall{ID: "call_123", Name: "get_weather", Input: map[string]interface{}{"location": "Tokyo"}}},
			},
		},
		NewToolResultMessage([]ToolResult{
			{ToolCallID: "call_123", Content: `{"temp": "20°C"}`, IsError: false},
		}),
	}
	result, err := convertMessagesToSDK(messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
}

func TestConvertMessagesUnknownRole(t *testing.T) {
	messages := []Message{{Role: "unknown", Content: []ContentBlock{{Type: BlockText, Text: "hi"}}}}
	_, err := convertMessagesToSDK(messages)
	if err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestConvertToolsToSDK(t *testing.T) {
	result := convertToolsToSDK(testTools, true)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].OfTool == nil {
		t.Fatal("expected OfTool to be set")
	}
	if result[0].OfTool.Name != "get_weather" {
		t.Fatalf("expected get_weather, got %s", result[0].OfTool.Name)
	}
	if !result[0].OfTool.Strict.Valid() || !result[0].OfTool.Strict.Value {
		t.Fatal("expected strict schema to be enabled")
	}
}

func TestConvertResponseFromSDKNativeThinking(t *testing.T) {
	msg := &anthropic.Message{
		StopReason: "end_turn",
		Usage: anthropic.Usage{
			InputTokens:  12,
			OutputTokens: 6,
		},
		Content: []anthropic.ContentBlockUnion{
			{Type: "thinking", Thinking: "hidden"},
			{Type: "text", Text: "visible"},
		},
	}

	resp := convertResponseFromSDK(msg, ReasoningParseModeNative)
	if resp.Text != "visible" {
		t.Fatalf("expected visible text, got %q", resp.Text)
	}
	if resp.ReasoningText != "hidden" {
		t.Fatalf("expected native thinking text, got %q", resp.ReasoningText)
	}
	if resp.InputTokens != 12 || resp.OutputTokens != 6 {
		t.Fatalf("unexpected token usage: %d/%d", resp.InputTokens, resp.OutputTokens)
	}
}

func TestConvertResponseFromSDKPreservesRawToolArguments(t *testing.T) {
	msg := &anthropic.Message{
		StopReason: "tool_use",
		Content: []anthropic.ContentBlockUnion{
			{Type: "tool_use", ID: "call_1", Name: "get_weather", Input: json.RawMessage("```json\n{\"location\":\"Tokyo\",}\n```")},
		},
	}

	resp := convertResponseFromSDK(msg, ReasoningParseModeNone)
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Input["location"] != "Tokyo" {
		t.Fatalf("expected repaired tool input, got %#v", resp.ToolCalls[0].Input)
	}
}

func TestResolveClaudeToolChoice(t *testing.T) {
	choice := resolveClaudeToolChoice(&ToolCallPolicy{
		Choice:          ToolChoiceSpecific,
		ToolName:        "get_weather",
		DisableParallel: true,
	})
	if choice.OfTool == nil {
		t.Fatalf("expected specific tool choice, got %#v", choice)
	}
	if choice.OfTool.Name != "get_weather" {
		t.Fatalf("expected get_weather, got %q", choice.OfTool.Name)
	}
	if !choice.OfTool.DisableParallelToolUse.Valid() || !choice.OfTool.DisableParallelToolUse.Value {
		t.Fatal("expected parallel tool use to be disabled")
	}
}

func TestNewTextMessage(t *testing.T) {
	msg := NewTextMessage(RoleUser, "hello")
	if msg.Role != RoleUser {
		t.Fatalf("expected user role, got %s", msg.Role)
	}
	if len(msg.Content) != 1 || msg.Content[0].Type != BlockText || msg.Content[0].Text != "hello" {
		t.Fatal("unexpected content")
	}
}

func TestNewToolResultMessage(t *testing.T) {
	msg := NewToolResultMessage([]ToolResult{
		{ToolCallID: "id1", Content: "result1", IsError: false},
		{ToolCallID: "id2", Content: "error", IsError: true},
	})
	if msg.Role != RoleTool {
		t.Fatalf("expected tool role, got %s", msg.Role)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(msg.Content))
	}
	if msg.Content[0].ToolCallID != "id1" {
		t.Fatalf("expected tool_call_id 'id1', got %s", msg.Content[0].ToolCallID)
	}
	if msg.Content[1].IsError != true {
		t.Fatal("expected second block to be error")
	}
}

func TestClaudeProviderName(t *testing.T) {
	p := NewClaudeProvider()
	if p.Name() != "claude" {
		t.Fatalf("expected 'claude', got %s", p.Name())
	}
}

func TestClaudeProviderEstimateTokens(t *testing.T) {
	p := NewClaudeProvider()
	msgs := []Message{NewTextMessage(RoleUser, "Hello world, this is a test.")}
	tokens := p.EstimateTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive token estimate, got %d", tokens)
	}
}

// --- Integration tests (require ANTHROPIC_API_KEY) ---

func TestClaudeChat(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	provider := NewClaudeProvider()
	resp, err := provider.Chat(context.Background(), ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "Say hello in one word.")},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Text == "" {
		t.Fatal("expected non-empty text")
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("expected end_turn, got %s", resp.StopReason)
	}
	t.Logf("Response: %s (tokens: %d/%d)", resp.Text, resp.InputTokens, resp.OutputTokens)
}

func TestClaudeChatToolUse(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	opts := []ClaudeOption{
		WithModel(anthropic.ModelClaudeSonnet4_6),
	}
	provider := NewClaudeProvider(opts...)
	resp, err := provider.Chat(context.Background(), ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "What is the weather in Tokyo?")},
		Tools:    testTools,
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.StopReason != StopReasonToolUse {
		t.Fatalf("expected tool_use, got %s", resp.StopReason)
	}
	if len(resp.ToolCalls) == 0 {
		t.Fatal("expected at least one tool call")
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "get_weather" {
		t.Fatalf("expected get_weather, got %s", tc.Name)
	}
	t.Logf("Tool call: %s(%v)", tc.Name, tc.Input)
}
