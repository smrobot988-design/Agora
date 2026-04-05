package orchestrator

import (
	"context"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
)

func TestOrchestratorImplementsRunner(t *testing.T) {
	var _ Runner = (*Orchestrator)(nil)
}

func TestOrchestratorName(t *testing.T) {
	o := NewOrchestrator("test-orch", nil, nil)
	if o.Name() != "test-orch" {
		t.Errorf("got %q, want %q", o.Name(), "test-orch")
	}
}

func TestRunnerNames(t *testing.T) {
	runners := []Runner{
		newMockRunner("a", ""),
		newMockRunner("b", ""),
		newMockRunner("c", ""),
	}
	names := runnerNames(runners)
	if len(names) != 3 || names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Errorf("got %v, want [a b c]", names)
	}
}

// simpleStrategy is a test Strategizer that returns the first runner's result.
type simpleStrategy struct{}

func (s *simpleStrategy) Orchestrate(ctx context.Context, input string, runners []Runner) (*core.Result, error) {
	return runners[0].Run(ctx, input)
}

func TestOrchestratorRun(t *testing.T) {
	r := newMockRunner("agent1", "hello from agent1")
	o := NewOrchestrator("test", &simpleStrategy{}, []Runner{r})

	result, err := o.Run(context.Background(), "test input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "hello from agent1" {
		t.Errorf("got %q, want %q", result.Text, "hello from agent1")
	}
	if len(r.calls) != 1 || r.calls[0] != "test input" {
		t.Errorf("calls = %v, want [\"test input\"]", r.calls)
	}
}
