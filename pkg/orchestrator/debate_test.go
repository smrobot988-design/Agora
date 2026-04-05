package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
)

func TestDefaultDebateFormatterFirstRound(t *testing.T) {
	f := &DefaultDebateFormatter{}
	prev := map[string]string{}
	result := f.Format(0, "a", "What is Go?", prev)
	if result != "What is Go?" {
		t.Errorf("first round = %q, want %q", result, "What is Go?")
	}
}

func TestDefaultDebateFormatterSubsequentRounds(t *testing.T) {
	f := &DefaultDebateFormatter{}
	prev := map[string]string{
		"b": "My opinion on Go",
		"c": "My take on Go",
	}
	result := f.Format(1, "a", "What is Go?", prev)
	// Should include original question and other opinions (sorted: b then c).
	if result == "What is Go?" {
		t.Error("should not return original question only")
	}
	if !contains(result, "What is Go?") {
		t.Error("missing original question")
	}
	if !contains(result, "b: My opinion on Go") {
		t.Error("missing runner b's opinion")
	}
	if !contains(result, "c: My take on Go") {
		t.Error("missing runner c's opinion")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDebateStrategizerEmpty(t *testing.T) {
	s := NewDebateStrategizer(3, &ConcatMerger{})
	result, err := s.Orchestrate(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "" {
		t.Errorf("got %q, want empty", result.Text)
	}
}

func TestDebateStrategizerSingleRound(t *testing.T) {
	a := newMockRunner("expert-a", "answer-a")
	s := NewDebateStrategizer(1, &ConcatMerger{Separator: ","})
	result, err := s.Orchestrate(context.Background(), "question", []Runner{a})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "answer-a" {
		t.Errorf("text = %q, want %q", result.Text, "answer-a")
	}
}

func TestDebateStrategizerMultiRound(t *testing.T) {
	// Use one runner that returns different responses per call (simulating round progression).
	r := &roundTripRunner{
		name:      "round-trip",
		callTexts: []string{"first take", "revised", "final position"},
	}

	s := NewDebateStrategizer(3, &ConcatMerger{Separator: " | "})
	result, err := s.Orchestrate(context.Background(), "question", []Runner{r})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The merger should get the final round's text (which is "final position").
	if result.Text != "final position" {
		t.Errorf("text = %q, want %q", result.Text, "final position")
	}
}

// roundTripRunner returns a different response for each Run call.
type roundTripRunner struct {
	name      string
	callTexts []string
	callIdx   int
}

func (r *roundTripRunner) Run(ctx context.Context, input string) (*core.Result, error) {
	idx := r.callIdx
	if idx >= len(r.callTexts) {
		idx = len(r.callTexts) - 1
	}
	r.callIdx++
	return &core.Result{Text: r.callTexts[idx], TotalInputTokens: 10, TotalOutputTokens: 5}, nil
}

func (r *roundTripRunner) Name() string { return r.name }

func TestDebateStrategizerMaxRoundsEnforced(t *testing.T) {
	a := newMockRunner("a", "response")
	b := newMockRunner("b", "response")

	s := NewDebateStrategizer(2, &ConcatMerger{})

	_, err := s.Orchestrate(context.Background(), "q", []Runner{a, b})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Each runner called twice (2 rounds).
	if len(a.calls) != 2 {
		t.Errorf("a calls = %d, want 2", len(a.calls))
	}
	if len(b.calls) != 2 {
		t.Errorf("b calls = %d, want 2", len(b.calls))
	}

	// Round 2 input should include other runner's response.
	if !contains(a.calls[1], "b: response") {
		t.Errorf("a round 2 input = %q, should mention b's response", a.calls[1])
	}
}

func TestDebateStrategizerError(t *testing.T) {
	a := newMockRunner("a", "fine")
	b := newErrorRunner("fail", fmt.Errorf("debate crashed"))

	s := NewDebateStrategizer(2, &ConcatMerger{})

	_, err := s.Orchestrate(context.Background(), "q", []Runner{a, b})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDebateStrategizerCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := newMockRunner("a", "never")
	s := NewDebateStrategizer(2, &ConcatMerger{})

	_, err := s.Orchestrate(ctx, "q", []Runner{r})
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}
