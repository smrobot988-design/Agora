package llm

import (
	"context"
	"os"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

// --- Interface compliance ---

var _ Provider = (*MiniMaxProvider)(nil)

// --- Unit tests ---

func TestMiniMaxProviderName(t *testing.T) {
	p := NewMiniMaxProvider()
	if p.Name() != "minimax" {
		t.Fatalf("expected 'minimax', got %s", p.Name())
	}
}

func TestMiniMaxProviderEstimateTokens(t *testing.T) {
	p := NewMiniMaxProvider()
	msgs := []Message{NewTextMessage(RoleUser, "Hello world, this is a test.")}
	tokens := p.EstimateTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive token estimate, got %d", tokens)
	}
}

func TestMiniMaxProviderChatStreamUnsupported(t *testing.T) {
	p := NewMiniMaxProvider()
	err := p.ChatStream(context.Background(), ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}, func(pr *PartialResponse) {})
	// MiniMax DOES support streaming, so this should not return ErrStreamingUnsupported
	if err == ErrStreamingUnsupported {
		t.Fatal("expected streaming to be supported by MiniMax provider")
	}
}

// --- Integration tests (require MINIMAX_API_KEY) ---

func TestMiniMaxChat(t *testing.T) {
	if os.Getenv("MINIMAX_API_KEY") == "" {
		t.Skip("MINIMAX_API_KEY not set")
	}
	provider := NewMiniMaxProvider()
	resp, err := provider.Chat(context.Background(), ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "告诉我你是什么，由谁开发，能干什么用？")},
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

func TestMiniMaxChatToolUse(t *testing.T) {
	if os.Getenv("MINIMAX_API_KEY") == "" {
		t.Skip("MINIMAX_API_KEY not set")
	}
	provider := NewMiniMaxProvider()
	tools := []schema.ToolDefinition{
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
	resp, err := provider.Chat(context.Background(), ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "What is the weather in Tokyo?")},
		Tools:    tools,
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

func TestMiniMaxChatStream(t *testing.T) {
	if os.Getenv("MINIMAX_API_KEY") == "" {
		t.Skip("MINIMAX_API_KEY not set")
	}
	provider := NewMiniMaxProvider()

	var textDelta string
	var toolCallEvents []*ToolCallDelta
	var stopReason StopReason

	err := provider.ChatStream(context.Background(), ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "用一句话介绍自己")},
	}, func(pr *PartialResponse) {
		if pr == nil {
			t.Log("stream ended")
			return
		}
		t.Logf("event: type=%s text_delta=%q stop_reason=%v tool_delta=%v usage_in=%d usage_out=%d",
			pr.Type, pr.TextDelta, pr.StopReason,
			pr.ToolCallDelta, pr.InputTokens, pr.OutputTokens)
		switch pr.Type {
		case StreamEventTextDelta:
			textDelta += pr.TextDelta
		case StreamEventToolDelta:
			if pr.ToolCallDelta != nil {
				toolCallEvents = append(toolCallEvents, pr.ToolCallDelta)
			}
		case StreamEventStop:
			stopReason = pr.StopReason
		}
	})

	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}
	if textDelta == "" {
		t.Fatal("expected at least one text delta event")
	}
	if stopReason != StopReasonEndTurn {
		t.Fatalf("expected stop_reason end_turn, got %s", stopReason)
	}
	if len(toolCallEvents) > 0 {
		t.Fatalf("expected no tool calls, got %d", len(toolCallEvents))
	}
	t.Logf("Streamed text: %s (len=%d)", textDelta, len(textDelta))
}
