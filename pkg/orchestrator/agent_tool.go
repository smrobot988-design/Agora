package orchestrator

import (
	"context"
	"fmt"

	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

// AgentTool wraps a Runner as a tool.Tool.
// When the LLM calls this tool, it executes the wrapped Runner with the
// provided task as input and returns Result.Text as the tool output.
//
// This enables two key patterns:
//   - Agent-as-Tool: register a sub-agent as a tool in another agent's registry.
//   - Supervisor: the supervisor agent's registry contains AgentTools for each sub-agent,
//     letting the LLM dynamically decide which sub-agent to invoke.
type AgentTool struct {
	name        string
	description string
	runner      Runner
}

// Compile-time interface check.
var _ tool.Tool = (*AgentTool)(nil)

// NewAgentTool creates a tool that delegates execution to the given Runner.
//
//   - name: the tool name visible to the LLM (e.g. "researcher", "coder").
//   - description: a description helping the LLM decide when to use this tool.
//   - runner: the Runner to invoke when the tool is called.
func NewAgentTool(name, description string, runner Runner) *AgentTool {
	return &AgentTool{name: name, description: description, runner: runner}
}

// Definition returns the tool schema for the LLM.
// The tool accepts a single "task" parameter describing what to delegate.
func (t *AgentTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.name,
		Description: t.description,
		InputSchema: schema.PropertySchema{
			Properties: map[string]interface{}{
				"task": map[string]interface{}{
					"type":        "string",
					"description": "The task to delegate to this agent",
				},
			},
			Required: []string{"task"},
		},
	}
}

// Execute runs the wrapped Runner with the task input and returns the result text.
func (t *AgentTool) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	task, _ := input["task"].(string)
	if task == "" {
		return "", fmt.Errorf("agent tool %q: task parameter is required", t.name)
	}

	result, err := t.runner.Run(ctx, task)
	if err != nil {
		return "", fmt.Errorf("agent tool %q: %w", t.name, err)
	}
	return result.Text, nil
}
