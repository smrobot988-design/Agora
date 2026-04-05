package orchestrator

import (
	"context"

	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/llm"
)

// Runner is the universal execution primitive for agentic tasks.
// Both single agents (via AgentRunner) and multi-agent orchestrations
// (via Orchestrator) implement this interface, enabling recursive composition.
type Runner interface {
	// Run executes the runner with the given input and returns a result.
	Run(ctx context.Context, input string) (*core.Result, error)

	// Name returns a human-readable identifier for this runner.
	Name() string
}

// RunnerWithToolResults is an optional interface that Runners can implement
// to expose tool execution results. This enables patterns like Swarm handoff
// detection without relying on LLM text parsing.
type RunnerWithToolResults interface {
	// LastToolResults returns the tool results from the most recent Run call.
	// Returns nil if the last run had no tool calls.
	LastToolResults() ([]llm.ToolResult, error)
}

// AgentRunner adapts a *core.Agent to the Runner interface.
// This is the bridge between the existing single-agent system and the
// orchestration layer, requiring zero modifications to core.Agent.
type AgentRunner struct {
	name          string
	agent         *core.Agent
	lastToolResults []llm.ToolResult // populated after each Run
}

// NewAgentRunner creates a Runner that wraps the given core.Agent.
func NewAgentRunner(name string, agent *core.Agent) *AgentRunner {
	return &AgentRunner{name: name, agent: agent}
}

// Run delegates to the underlying Agent.Run and captures tool results.
func (r *AgentRunner) Run(ctx context.Context, input string) (*core.Result, error) {
	result, err := r.agent.Run(ctx, input)
	if err != nil {
		return result, err
	}
	r.lastToolResults, _ = r.agent.LastToolResults()
	return result, nil
}

// Name returns the runner's name.
func (r *AgentRunner) Name() string { return r.name }

// LastToolResults returns tool results captured from the most recent Run.
func (r *AgentRunner) LastToolResults() ([]llm.ToolResult, error) {
	return r.lastToolResults, nil
}
