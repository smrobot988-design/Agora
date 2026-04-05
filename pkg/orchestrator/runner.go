package orchestrator

import (
	"context"

	"github.com/smrobot988-design/Agora/pkg/core"
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

// AgentRunner adapts a *core.Agent to the Runner interface.
// This is the bridge between the existing single-agent system and the
// orchestration layer, requiring zero modifications to core.Agent.
type AgentRunner struct {
	name  string
	agent *core.Agent
}

// NewAgentRunner creates a Runner that wraps the given core.Agent.
func NewAgentRunner(name string, agent *core.Agent) *AgentRunner {
	return &AgentRunner{name: name, agent: agent}
}

// Run delegates to the underlying Agent.Run.
func (r *AgentRunner) Run(ctx context.Context, input string) (*core.Result, error) {
	return r.agent.Run(ctx, input)
}

// Name returns the runner's name.
func (r *AgentRunner) Name() string { return r.name }
