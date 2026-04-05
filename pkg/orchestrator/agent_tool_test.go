package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/tool"
)

func TestAgentToolImplementsTool(t *testing.T) {
	var _ tool.Tool = (*AgentTool)(nil)
}

func TestAgentToolDefinition(t *testing.T) {
	at := NewAgentTool("researcher", "Searches for information", newMockRunner("r", ""))
	def := at.Definition()

	if def.Name != "researcher" {
		t.Errorf("name = %q, want %q", def.Name, "researcher")
	}
	if def.Description != "Searches for information" {
		t.Errorf("description = %q, want %q", def.Description, "Searches for information")
	}
	if len(def.InputSchema.Required) != 1 || def.InputSchema.Required[0] != "task" {
		t.Errorf("required = %v, want [task]", def.InputSchema.Required)
	}
}

func TestAgentToolExecute(t *testing.T) {
	runner := newMockRunner("agent", "agent response")
	at := NewAgentTool("agent", "test agent", runner)

	output, err := at.Execute(context.Background(), map[string]interface{}{
		"task": "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "agent response" {
		t.Errorf("output = %q, want %q", output, "agent response")
	}
	if len(runner.calls) != 1 || runner.calls[0] != "do something" {
		t.Errorf("calls = %v, want [\"do something\"]", runner.calls)
	}
}

func TestAgentToolExecuteEmptyTask(t *testing.T) {
	at := NewAgentTool("agent", "test", newMockRunner("a", ""))
	_, err := at.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Error("expected error for empty task")
	}
}

func TestAgentToolExecuteRunnerError(t *testing.T) {
	runner := newErrorRunner("fail", fmt.Errorf("agent failed"))
	at := NewAgentTool("fail", "test", runner)

	_, err := at.Execute(context.Background(), map[string]interface{}{
		"task": "do something",
	})
	if err == nil {
		t.Error("expected error from runner")
	}
}
