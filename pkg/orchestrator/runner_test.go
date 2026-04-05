package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
)

// mockRunner is a test double for Runner.
type mockRunner struct {
	name   string
	result *core.Result
	err    error
	calls  []string // records inputs passed to Run
}

func (m *mockRunner) Run(ctx context.Context, input string) (*core.Result, error) {
	m.calls = append(m.calls, input)
	return m.result, m.err
}

func (m *mockRunner) Name() string { return m.name }

func newMockRunner(name, text string) *mockRunner {
	return &mockRunner{
		name:   name,
		result: &core.Result{Text: text},
	}
}

func newErrorRunner(name string, err error) *mockRunner {
	return &mockRunner{
		name: name,
		err:  err,
	}
}

func TestAgentRunnerImplementsRunner(t *testing.T) {
	// Compile-time check that AgentRunner implements Runner.
	var _ Runner = (*AgentRunner)(nil)
}

func TestMockRunnerRun(t *testing.T) {
	r := newMockRunner("test", "hello")
	result, err := r.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "hello" {
		t.Errorf("got %q, want %q", result.Text, "hello")
	}
	if len(r.calls) != 1 || r.calls[0] != "input" {
		t.Errorf("calls = %v, want [\"input\"]", r.calls)
	}
}

func TestMockRunnerError(t *testing.T) {
	r := newErrorRunner("fail", fmt.Errorf("boom"))
	_, err := r.Run(context.Background(), "input")
	if err == nil || err.Error() != "boom" {
		t.Errorf("got err = %v, want boom", err)
	}
}

func TestMockRunnerName(t *testing.T) {
	r := newMockRunner("myrunner", "")
	if r.Name() != "myrunner" {
		t.Errorf("got %q, want %q", r.Name(), "myrunner")
	}
}
