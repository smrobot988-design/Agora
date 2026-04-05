package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

func TestSwarmRunnerImplementsRunner(t *testing.T) {
	var _ Runner = (*SwarmRunner)(nil)
}

func TestHandoffToolImplementsTool(t *testing.T) {
	var _ tool.Tool = (*HandoffTool)(nil)
}

func TestParseHandoffFromContent(t *testing.T) {
	tests := []struct {
		input   string
		target  string
		context string
	}{
		{"HANDOFF:tech:please help", "tech", "please help"},
		{"HANDOFF:billing:", "billing", ""},
		{"HANDOFF:billing", "billing", ""},
	}

	for _, tt := range tests {
		target, ctx := parseHandoffFromContent(tt.input)
		if target != tt.target || ctx != tt.context {
			t.Errorf("parseHandoffFromContent(%q) = (%q, %q), want (%q, %q)",
				tt.input, target, ctx, tt.target, tt.context)
		}
	}
}

func TestSwarmRunnerNoHandoff(t *testing.T) {
	// Agent returns a normal response with no handoff in tool results.
	agent := &toolResultsMockRunner{
		name:     "front_desk",
		result:   &core.Result{Text: "I can help you with that!", TotalInputTokens: 10, TotalOutputTokens: 5},
		toolResults: nil, // no handoff
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
	// front_desk has a handoff tool result; tech_support returns final answer.
	frontDesk := &toolResultsMockRunner{
		name:   "front_desk",
		result: &core.Result{Text: "Let me transfer you...", TotalInputTokens: 10, TotalOutputTokens: 5},
		toolResults: []llm.ToolResult{
			{ToolCallID: "1", Content: "HANDOFF:tech_support:user has a bug"},
		},
	}
	techSupport := &toolResultsMockRunner{
		name:        "tech_support",
		result:      &core.Result{Text: "Bug fixed!", TotalInputTokens: 20, TotalOutputTokens: 10},
		toolResults: nil,
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
	a := &toolResultsMockRunner{
		name:   "a",
		result: &core.Result{Text: "transferring..."},
		toolResults: []llm.ToolResult{
			{ToolCallID: "1", Content: "HANDOFF:b:from a"},
		},
	}
	b := &toolResultsMockRunner{
		name:   "b",
		result: &core.Result{Text: "transferring..."},
		toolResults: []llm.ToolResult{
			{ToolCallID: "2", Content: "HANDOFF:c:from b"},
		},
	}
	c := &toolResultsMockRunner{
		name:        "c",
		result:      &core.Result{Text: "final"},
		toolResults: nil,
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
	a := &toolResultsMockRunner{
		name:   "a",
		result: &core.Result{Text: "loop"},
		toolResults: []llm.ToolResult{
			{ToolCallID: "1", Content: "HANDOFF:b:loop"},
		},
	}
	b := &toolResultsMockRunner{
		name:   "b",
		result: &core.Result{Text: "loop"},
		toolResults: []llm.ToolResult{
			{ToolCallID: "2", Content: "HANDOFF:a:loop"},
		},
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
	a := &toolResultsMockRunner{
		name:   "a",
		result: &core.Result{Text: "..."},
		toolResults: []llm.ToolResult{
			{ToolCallID: "1", Content: "HANDOFF:nonexistent:help"},
		},
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

	a := &toolResultsMockRunner{
		name:   "a",
		result: &core.Result{Text: "never"},
	}
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

// toolResultsMockRunner is a mock that implements RunnerWithToolResults.
type toolResultsMockRunner struct {
	name        string
	result      *core.Result
	toolResults []llm.ToolResult
	calls      []string // records inputs passed to Run
}

func (m *toolResultsMockRunner) Run(ctx context.Context, input string) (*core.Result, error) {
	m.calls = append(m.calls, input)
	return m.result, nil
}

func (m *toolResultsMockRunner) Name() string { return m.name }

func (m *toolResultsMockRunner) LastToolResults() ([]llm.ToolResult, error) {
	return m.toolResults, nil
}
