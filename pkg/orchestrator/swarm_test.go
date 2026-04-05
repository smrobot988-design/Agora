package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

func TestSwarmRunnerImplementsRunner(t *testing.T) {
	var _ Runner = (*SwarmRunner)(nil)
}

func TestHandoffToolImplementsTool(t *testing.T) {
	var _ tool.Tool = (*HandoffTool)(nil)
}

func TestParseHandoff(t *testing.T) {
	tests := []struct {
		input     string
		target    string
		message   string
		isHandoff bool
	}{
		{"HANDOFF:tech:please help", "tech", "please help", true},
		{"HANDOFF:billing:", "billing", "", true},
		{"HANDOFF:billing", "billing", "", true},
		{"normal response", "", "", false},
		{"", "", "", false},
		{"HANDOFF:", "", "", true},
	}

	for _, tt := range tests {
		target, message, isHandoff := parseHandoff(tt.input)
		if target != tt.target || message != tt.message || isHandoff != tt.isHandoff {
			t.Errorf("parseHandoff(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.input, target, message, isHandoff, tt.target, tt.message, tt.isHandoff)
		}
	}
}

func TestSwarmRunnerNoHandoff(t *testing.T) {
	// Agent returns a normal response (no handoff).
	agent := &mockRunner{
		name:   "front_desk",
		result: &core.Result{Text: "I can help you with that!", TotalInputTokens: 10, TotalOutputTokens: 5},
	}

	swarm := NewSwarmRunner("test-swarm", "front_desk", map[string]Runner{
		"front_desk": agent,
	})

	result, err := swarm.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "I can help you with that!" {
		t.Errorf("text = %q, want direct response", result.Text)
	}
}

func TestSwarmRunnerSingleHandoff(t *testing.T) {
	// front_desk hands off to tech_support, which returns a final answer.
	frontDesk := &mockRunner{
		name:   "front_desk",
		result: &core.Result{Text: "HANDOFF:tech_support:user has a bug", TotalInputTokens: 10, TotalOutputTokens: 5},
	}
	techSupport := &mockRunner{
		name:   "tech_support",
		result: &core.Result{Text: "Bug fixed!", TotalInputTokens: 20, TotalOutputTokens: 10},
	}

	swarm := NewSwarmRunner("test-swarm", "front_desk", map[string]Runner{
		"front_desk":   frontDesk,
		"tech_support": techSupport,
	})

	result, err := swarm.Run(context.Background(), "I found a bug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Bug fixed!" {
		t.Errorf("text = %q, want %q", result.Text, "Bug fixed!")
	}
	// Token usage aggregated.
	if result.TotalInputTokens != 30 {
		t.Errorf("input tokens = %d, want 30", result.TotalInputTokens)
	}
	// tech_support received the handoff context.
	if techSupport.calls[0] != "user has a bug" {
		t.Errorf("tech input = %q, want %q", techSupport.calls[0], "user has a bug")
	}
}

func TestSwarmRunnerChainedHandoffs(t *testing.T) {
	a := &mockRunner{
		name:   "a",
		result: &core.Result{Text: "HANDOFF:b:from a"},
	}
	b := &mockRunner{
		name:   "b",
		result: &core.Result{Text: "HANDOFF:c:from b"},
	}
	c := &mockRunner{
		name:   "c",
		result: &core.Result{Text: "final"},
	}

	swarm := NewSwarmRunner("chain", "a", map[string]Runner{
		"a": a, "b": b, "c": c,
	})

	result, err := swarm.Run(context.Background(), "start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "final" {
		t.Errorf("text = %q, want %q", result.Text, "final")
	}
}

func TestSwarmRunnerMaxHandoffs(t *testing.T) {
	// Create an infinite loop of handoffs.
	a := &mockRunner{
		name:   "a",
		result: &core.Result{Text: "HANDOFF:b:loop"},
	}
	b := &mockRunner{
		name:   "b",
		result: &core.Result{Text: "HANDOFF:a:loop"},
	}

	swarm := NewSwarmRunner("loop", "a", map[string]Runner{
		"a": a, "b": b,
	}, WithMaxHandoffs(3))

	_, err := swarm.Run(context.Background(), "start")
	if err == nil {
		t.Fatal("expected max handoffs error")
	}
}

func TestSwarmRunnerUnknownAgent(t *testing.T) {
	a := &mockRunner{
		name:   "a",
		result: &core.Result{Text: "HANDOFF:nonexistent:help"},
	}

	swarm := NewSwarmRunner("test", "a", map[string]Runner{"a": a})

	_, err := swarm.Run(context.Background(), "start")
	if err == nil {
		t.Fatal("expected unknown agent error")
	}
}

func TestHandoffToolDefinition(t *testing.T) {
	ht := NewHandoffTool([]string{"tech", "billing"})
	def := ht.Definition()

	if def.Name != "handoff" {
		t.Errorf("name = %q, want %q", def.Name, "handoff")
	}
	if len(def.InputSchema.Required) != 2 {
		t.Errorf("required = %v, want 2 fields", def.InputSchema.Required)
	}
}

func TestHandoffToolExecute(t *testing.T) {
	ht := NewHandoffTool([]string{"tech"})
	output, err := ht.Execute(context.Background(), map[string]interface{}{
		"target_agent": "tech",
		"context":      "user needs help",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "HANDOFF:tech:user needs help" {
		t.Errorf("output = %q, want %q", output, "HANDOFF:tech:user needs help")
	}
}

func TestHandoffToolExecuteNoTarget(t *testing.T) {
	ht := NewHandoffTool([]string{"tech"})
	_, err := ht.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing target_agent")
	}
}

func TestSwarmRunnerCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := newMockRunner("a", "never")
	swarm := NewSwarmRunner("test", "a", map[string]Runner{"a": a})

	_, err := swarm.Run(ctx, "input")
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}

func TestSwarmRunnerAgentError(t *testing.T) {
	a := newErrorRunner("a", fmt.Errorf("agent crashed"))
	swarm := NewSwarmRunner("test", "a", map[string]Runner{"a": a})

	_, err := swarm.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("expected error from agent")
	}
}
