package llm

import (
	"context"
	"os"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

var _ Provider = (*GLMProvider)(nil)

func TestGLMProviderName(t *testing.T) {
	p := NewGLMProvider()
	if p.Name() != "glm" {
		t.Fatalf("expected 'glm', got %s", p.Name())
	}
}

func TestGLMProviderEstimateTokens(t *testing.T) {
	p := NewGLMProvider()
	msgs := []Message{NewTextMessage(RoleUser, "Hello world, this is a test.")}
	tokens := p.EstimateTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive token estimate, got %d", tokens)
	}
}

// --- Integration tests (require GLM_API_KEY) ---

func TestGLMChat(t *testing.T) {
	if os.Getenv("GLM_API_KEY") == "" {
		t.Skip("GLM_API_KEY not set")
	}
	provider := NewGLMProvider()
	resp, err := provider.Chat(context.Background(), ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "Say hello in one sentence.")},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Text == "" {
		t.Fatal("expected non-empty text")
	}
	t.Logf("Response: %s (tokens: %d/%d)", resp.Text, resp.InputTokens, resp.OutputTokens)
}

func TestGLMChatToolUse(t *testing.T) {
	if os.Getenv("GLM_API_KEY") == "" {
		t.Skip("GLM_API_KEY not set")
	}
	provider := NewGLMProvider()
	tools := []schema.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a location",
			InputSchema: schema.PropertySchema{
				Properties: map[string]interface{}{
					"location": map[string]interface{}{"type": "string", "description": "City name"},
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
	t.Logf("Tool call: %s(%v)", resp.ToolCalls[0].Name, resp.ToolCalls[0].Input)
}

func TestGLMChatStream(t *testing.T) {
	if os.Getenv("GLM_API_KEY") == "" {
		t.Skip("GLM_API_KEY not set")
	}
	provider := NewGLMProvider()

	var textDelta string
	var stopReason StopReason

	err := provider.ChatStream(context.Background(), ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "Say hello in one sentence.")},
	}, func(pr *PartialResponse) {
		if pr == nil {
			return
		}
		switch pr.Type {
		case StreamEventTextDelta:
			textDelta += pr.TextDelta
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
	t.Logf("Streamed text: %s (stop_reason=%s)", textDelta, stopReason)
}
