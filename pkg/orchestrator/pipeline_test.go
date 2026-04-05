package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
)

func TestPipelineStrategizerEmpty(t *testing.T) {
	s := NewPipelineStrategizer()
	result, err := s.Orchestrate(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "" {
		t.Errorf("got %q, want empty", result.Text)
	}
}

func TestPipelineStrategizerSingle(t *testing.T) {
	r := newMockRunner("only", "processed")
	s := NewPipelineStrategizer()

	result, err := s.Orchestrate(context.Background(), "input", []Runner{r})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "processed" {
		t.Errorf("got %q, want %q", result.Text, "processed")
	}
	if r.calls[0] != "input" {
		t.Errorf("runner received %q, want %q", r.calls[0], "input")
	}
}

func TestPipelineStrategizerChaining(t *testing.T) {
	// Each runner appends its name to the input.
	r1 := &mockRunner{
		name:   "step1",
		result: &core.Result{Text: "step1-output", TotalInputTokens: 10, TotalOutputTokens: 5, Turns: 1},
	}
	r2 := &mockRunner{
		name:   "step2",
		result: &core.Result{Text: "step2-output", TotalInputTokens: 20, TotalOutputTokens: 10, Turns: 2},
	}
	r3 := &mockRunner{
		name:   "step3",
		result: &core.Result{Text: "final-output", TotalInputTokens: 15, TotalOutputTokens: 8, Turns: 1},
	}

	s := NewPipelineStrategizer()
	result, err := s.Orchestrate(context.Background(), "start", []Runner{r1, r2, r3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Final output is from last stage.
	if result.Text != "final-output" {
		t.Errorf("text = %q, want %q", result.Text, "final-output")
	}

	// Each runner receives the previous runner's output.
	if r1.calls[0] != "start" {
		t.Errorf("r1 input = %q, want %q", r1.calls[0], "start")
	}
	if r2.calls[0] != "step1-output" {
		t.Errorf("r2 input = %q, want %q", r2.calls[0], "step1-output")
	}
	if r3.calls[0] != "step2-output" {
		t.Errorf("r3 input = %q, want %q", r3.calls[0], "step2-output")
	}

	// Token usage is aggregated.
	if result.TotalInputTokens != 45 {
		t.Errorf("input tokens = %d, want 45", result.TotalInputTokens)
	}
	if result.TotalOutputTokens != 23 {
		t.Errorf("output tokens = %d, want 23", result.TotalOutputTokens)
	}
	if result.Turns != 4 {
		t.Errorf("turns = %d, want 4", result.Turns)
	}
}

func TestPipelineStrategizerError(t *testing.T) {
	r1 := newMockRunner("ok", "fine")
	r2 := newErrorRunner("fail", fmt.Errorf("stage failed"))

	s := NewPipelineStrategizer()
	_, err := s.Orchestrate(context.Background(), "input", []Runner{r1, r2})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPipelineStrategizerCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	r := newMockRunner("never", "never")
	s := NewPipelineStrategizer()

	_, err := s.Orchestrate(ctx, "input", []Runner{r})
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}

// testTransformer adds a prefix to the output between stages.
type testTransformer struct{}

func (tt *testTransformer) Transform(stageIndex int, runnerName string, result *core.Result) string {
	return fmt.Sprintf("[from %s] %s", runnerName, result.Text)
}

func TestPipelineStrategizerWithTransformer(t *testing.T) {
	r1 := newMockRunner("step1", "data")
	r2 := newMockRunner("step2", "final")

	s := NewPipelineStrategizer(WithTransformer(&testTransformer{}))
	_, err := s.Orchestrate(context.Background(), "start", []Runner{r1, r2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// r2 should receive transformed input.
	if r2.calls[0] != "[from step1] data" {
		t.Errorf("r2 input = %q, want %q", r2.calls[0], "[from step1] data")
	}
}
