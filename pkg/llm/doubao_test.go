package llm

import (
	"context"
	"os"
	"testing"
)

var _ Provider = (*DoubaoProvider)(nil)

func TestDoubaoProviderName(t *testing.T) {
	p := NewDoubaoProvider()
	if p.Name() != "doubao" {
		t.Fatalf("expected 'doubao', got %s", p.Name())
	}
}

func TestDoubaoProviderEstimateTokens(t *testing.T) {
	p := NewDoubaoProvider()
	msgs := []Message{NewTextMessage(RoleUser, "Hello world, this is a test.")}
	tokens := p.EstimateTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive token estimate, got %d", tokens)
	}
}

// --- Integration tests (require ARK_API_KEY and ARK_MODEL) ---

func TestDoubaoChat(t *testing.T) {
	if os.Getenv("ARK_API_KEY") == "" || os.Getenv("ARK_MODEL") == "" {
		t.Skip("ARK_API_KEY or ARK_MODEL not set")
	}
	provider := NewDoubaoProvider(
		DoubaoWithModel(os.Getenv("ARK_MODEL")),
	)
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

func TestDoubaoChatStream(t *testing.T) {
	if os.Getenv("ARK_API_KEY") == "" || os.Getenv("ARK_MODEL") == "" {
		t.Skip("ARK_API_KEY or ARK_MODEL not set")
	}
	provider := NewDoubaoProvider(
		DoubaoWithModel(os.Getenv("ARK_MODEL")),
	)

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
