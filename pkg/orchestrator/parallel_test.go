package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
)

func TestParallelStrategizerEmpty(t *testing.T) {
	s := NewParallelStrategizer(&ConcatMerger{})
	result, err := s.Orchestrate(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "" {
		t.Errorf("got %q, want empty", result.Text)
	}
}

func TestParallelStrategizerConcurrent(t *testing.T) {
	r1 := &mockRunner{
		name:   "a",
		result: &core.Result{Text: "alpha", TotalInputTokens: 10, TotalOutputTokens: 5},
	}
	r2 := &mockRunner{
		name:   "b",
		result: &core.Result{Text: "beta", TotalInputTokens: 20, TotalOutputTokens: 10},
	}

	s := NewParallelStrategizer(&ConcatMerger{Separator: ","})
	result, err := s.Orchestrate(context.Background(), "input", []Runner{r1, r2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both runners received the same input.
	if len(r1.calls) != 1 || r1.calls[0] != "input" {
		t.Errorf("r1 calls = %v, want [input]", r1.calls)
	}
	if len(r2.calls) != 1 || r2.calls[0] != "input" {
		t.Errorf("r2 calls = %v, want [input]", r2.calls)
	}

	// Results merged.
	if result.Text != "alpha,beta" {
		t.Errorf("text = %q, want %q", result.Text, "alpha,beta")
	}
	if result.TotalInputTokens != 30 {
		t.Errorf("input tokens = %d, want 30", result.TotalInputTokens)
	}
}

func TestParallelStrategizerError(t *testing.T) {
	r1 := newMockRunner("ok", "fine")
	r2 := newErrorRunner("fail", fmt.Errorf("runner failed"))

	s := NewParallelStrategizer(&ConcatMerger{})
	_, err := s.Orchestrate(context.Background(), "input", []Runner{r1, r2})
	if err == nil {
		t.Fatal("expected error from failing runner")
	}
}

func TestParallelStrategizerMaxWorkers(t *testing.T) {
	makeRunner := func(name string) Runner {
		return &mockRunner{
			name: name,
			result: &core.Result{Text: name},
		}
	}

	// With 5 runners and maxWorkers=2, at most 2 should run concurrently.
	// Note: this test is inherently probabilistic but validates the option is accepted.
	runners := []Runner{
		makeRunner("a"), makeRunner("b"), makeRunner("c"),
		makeRunner("d"), makeRunner("e"),
	}

	s := NewParallelStrategizer(&ConcatMerger{}, WithMaxWorkers(2))
	result, err := s.Orchestrate(context.Background(), "in", runners)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text == "" {
		t.Error("expected non-empty merged text")
	}
}

func TestParallelStrategizerCancelled(t *testing.T) {
	// Use a runner that respects context cancellation and returns an error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &contextAwareRunner{
		name: "cancelled",
		err:  context.Canceled,
	}

	s := NewParallelStrategizer(&ConcatMerger{})

	_, err := s.Orchestrate(ctx, "input", []Runner{r})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// contextAwareRunner is a mock that returns a context error.
type contextAwareRunner struct {
	name string
	err  error
}

func (r *contextAwareRunner) Run(ctx context.Context, input string) (*core.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &core.Result{Text: "done"}, nil
}

func (r *contextAwareRunner) Name() string { return r.name }
