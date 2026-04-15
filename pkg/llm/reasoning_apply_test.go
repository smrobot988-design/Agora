package llm

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestResolveOpenAICompatReasoningKeepsProviderDefaults(t *testing.T) {
	resolution := resolveOpenAICompatReasoning("minimax", "MiniMax-M1", ReasoningParseModeThinkTag, nil)

	if resolution.model != "MiniMax-M1" {
		t.Fatalf("expected model to stay unchanged, got %q", resolution.model)
	}
	if resolution.parseMode != ReasoningParseModeThinkTag {
		t.Fatalf("expected default parse mode think_tag, got %s", resolution.parseMode)
	}
	if resolution.applied == nil {
		t.Fatal("expected applied reasoning metadata")
	}
	if resolution.applied.Source != "provider_default" {
		t.Fatalf("expected provider_default source, got %q", resolution.applied.Source)
	}
	if resolution.applied.ParseMode != ReasoningParseModeThinkTag {
		t.Fatalf("expected applied parse mode think_tag, got %s", resolution.applied.ParseMode)
	}
}

func TestResolveOpenAICompatReasoningGPTMapsEffort(t *testing.T) {
	resolution := resolveOpenAICompatReasoning("gpt", "gpt-5", ReasoningParseModeNone, &ReasoningConfig{
		Mode:   ThinkingModeOn,
		Effort: ThinkingEffortMax,
	})

	if resolution.model != "gpt-5" {
		t.Fatalf("expected gpt model to stay unchanged, got %q", resolution.model)
	}
	if resolution.reasoningEffort != "high" {
		t.Fatalf("expected max to downgrade to high, got %q", resolution.reasoningEffort)
	}
	if resolution.parseMode != ReasoningParseModeNative {
		t.Fatalf("expected native parse mode, got %s", resolution.parseMode)
	}
	if resolution.applied == nil || resolution.applied.Source != "request" {
		t.Fatalf("expected request source, got %#v", resolution.applied)
	}
	if resolution.applied.Effort != ThinkingEffortHigh {
		t.Fatalf("expected applied effort high, got %s", resolution.applied.Effort)
	}
	if !containsNote(resolution.applied.Notes, "downgraded to high") {
		t.Fatalf("expected downgrade note, got %v", resolution.applied.Notes)
	}
}

func TestResolveOpenAICompatReasoningDeepSeekSwitchesModel(t *testing.T) {
	resolution := resolveOpenAICompatReasoning("deepseek", "deepseek-reasoner", ReasoningParseModeNone, &ReasoningConfig{
		Mode: ThinkingModeOff,
	})

	if resolution.model != "deepseek-chat" {
		t.Fatalf("expected deepseek-chat model, got %q", resolution.model)
	}
	if resolution.parseMode != ReasoningParseModeNone {
		t.Fatalf("expected parse mode none, got %s", resolution.parseMode)
	}
	if resolution.applied == nil || resolution.applied.Model != "deepseek-chat" {
		t.Fatalf("expected applied model deepseek-chat, got %#v", resolution.applied)
	}
}

func TestResolveClaudeReasoningKeepsProviderDefaults(t *testing.T) {
	resolution := resolveClaudeReasoning(anthropic.ModelClaudeSonnet4_6, nil)

	if resolution.useThinking {
		t.Fatal("expected provider default thinking to be preserved")
	}
	if resolution.useOutput {
		t.Fatal("expected no explicit output config when no overrides are set")
	}
	if resolution.parseMode != ReasoningParseModeNone {
		t.Fatalf("expected default parse mode none, got %s", resolution.parseMode)
	}
	if resolution.applied == nil || resolution.applied.Source != "provider_default" {
		t.Fatalf("expected provider_default source, got %#v", resolution.applied)
	}
}

func TestResolveClaudeReasoningAppliesThinkingOverrides(t *testing.T) {
	resolution := resolveClaudeReasoning(anthropic.ModelClaudeSonnet4_6, &ReasoningConfig{
		Mode:   ThinkingModeOn,
		Effort: ThinkingEffortHigh,
	})

	if !resolution.useThinking {
		t.Fatal("expected explicit thinking config to be sent")
	}
	if !resolution.useOutput {
		t.Fatal("expected effort output config to be sent")
	}
	if resolution.parseMode != ReasoningParseModeNative {
		t.Fatalf("expected native parse mode, got %s", resolution.parseMode)
	}
	if resolution.applied == nil {
		t.Fatal("expected applied reasoning metadata")
	}
	if resolution.applied.Source != "request" {
		t.Fatalf("expected request source, got %q", resolution.applied.Source)
	}
	if resolution.applied.BudgetTokens != int(claudeDefaultThinkingBudget) {
		t.Fatalf("expected default thinking budget %d, got %d", claudeDefaultThinkingBudget, resolution.applied.BudgetTokens)
	}
	if resolution.applied.Effort != ThinkingEffortHigh {
		t.Fatalf("expected applied effort high, got %s", resolution.applied.Effort)
	}
	if !containsNote(resolution.applied.Notes, "defaulted to 1024") {
		t.Fatalf("expected default budget note, got %v", resolution.applied.Notes)
	}
}

func containsNote(notes []string, want string) bool {
	for _, note := range notes {
		if strings.Contains(note, want) {
			return true
		}
	}
	return false
}
