package orchestrator

import (
	"context"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
)

func TestSupervisorStrategizer(t *testing.T) {
	// The supervisor runner simulates an agent that calls sub-agents via tools.
	supervisor := &mockRunner{
		name:   "supervisor",
		result: &core.Result{Text: "supervisor decided: done", TotalInputTokens: 100, TotalOutputTokens: 50, Turns: 3},
	}

	s := NewSupervisorStrategizer(supervisor)

	// The runners slice is unused by supervisor (sub-agents are in the tool registry).
	result, err := s.Orchestrate(context.Background(), "complex task", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "supervisor decided: done" {
		t.Errorf("text = %q, want %q", result.Text, "supervisor decided: done")
	}
	if result.TotalInputTokens != 100 {
		t.Errorf("input tokens = %d, want 100", result.TotalInputTokens)
	}
	if len(supervisor.calls) != 1 || supervisor.calls[0] != "complex task" {
		t.Errorf("calls = %v, want [\"complex task\"]", supervisor.calls)
	}
}

func TestSupervisorStrategizerError(t *testing.T) {
	supervisor := newErrorRunner("fail-supervisor", context.DeadlineExceeded)
	s := NewSupervisorStrategizer(supervisor)

	_, err := s.Orchestrate(context.Background(), "input", nil)
	if err == nil {
		t.Fatal("expected error from supervisor")
	}
}
