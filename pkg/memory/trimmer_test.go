package memory

import (
	"testing"

	"github.com/smrobot988-design/Agora/pkg/llm"
)

func TestNoOpTrimmer(t *testing.T) {
	trimmer := &NoOpTrimmer{}
	msgs := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "1"),
		llm.NewTextMessage(llm.RoleAssistant, "2"),
		llm.NewTextMessage(llm.RoleUser, "3"),
	}
	result := trimmer.Trim(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
}

func TestSlidingWindowTrimmer(t *testing.T) {
	trimmer := &SlidingWindowTrimmer{MaxMessages: 2}
	msgs := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "1"),
		llm.NewTextMessage(llm.RoleAssistant, "2"),
		llm.NewTextMessage(llm.RoleUser, "3"),
		llm.NewTextMessage(llm.RoleAssistant, "4"),
	}
	result := trimmer.Trim(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Content[0].Text != "3" {
		t.Fatalf("expected '3', got %q", result[0].Content[0].Text)
	}
	if result[1].Content[0].Text != "4" {
		t.Fatalf("expected '4', got %q", result[1].Content[0].Text)
	}
}

func TestSlidingWindowTrimmerUnderLimit(t *testing.T) {
	trimmer := &SlidingWindowTrimmer{MaxMessages: 10}
	msgs := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "1"),
		llm.NewTextMessage(llm.RoleAssistant, "2"),
	}
	result := trimmer.Trim(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
}

func TestSlidingWindowPreserveFirst(t *testing.T) {
	trimmer := &SlidingWindowTrimmer{MaxMessages: 3, PreserveFirst: true}
	msgs := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "task"),
		llm.NewTextMessage(llm.RoleAssistant, "2"),
		llm.NewTextMessage(llm.RoleUser, "3"),
		llm.NewTextMessage(llm.RoleAssistant, "4"),
		llm.NewTextMessage(llm.RoleUser, "5"),
	}
	result := trimmer.Trim(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if result[0].Content[0].Text != "task" {
		t.Fatalf("expected first message preserved, got %q", result[0].Content[0].Text)
	}
	if result[1].Content[0].Text != "4" {
		t.Fatalf("expected '4', got %q", result[1].Content[0].Text)
	}
	if result[2].Content[0].Text != "5" {
		t.Fatalf("expected '5', got %q", result[2].Content[0].Text)
	}
}

func TestTokenBudgetTrimmer(t *testing.T) {
	estimator := func(msgs []llm.Message) int { return len(msgs) * 100 }
	trimmer := &TokenBudgetTrimmer{MaxTokens: 250, EstimateTokens: estimator}

	msgs := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "1"),
		llm.NewTextMessage(llm.RoleAssistant, "2"),
		llm.NewTextMessage(llm.RoleUser, "3"),
		llm.NewTextMessage(llm.RoleAssistant, "4"),
	}
	result := trimmer.Trim(msgs)
	if len(result) > 2 {
		t.Fatalf("expected at most 2 messages to fit budget, got %d", len(result))
	}
}

func TestTokenBudgetTrimmerUnderBudget(t *testing.T) {
	estimator := func(msgs []llm.Message) int { return len(msgs) * 10 }
	trimmer := &TokenBudgetTrimmer{MaxTokens: 1000, EstimateTokens: estimator}

	msgs := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "1"),
		llm.NewTextMessage(llm.RoleAssistant, "2"),
	}
	result := trimmer.Trim(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (under budget), got %d", len(result))
	}
}
