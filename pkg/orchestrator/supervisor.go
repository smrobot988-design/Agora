package orchestrator

import (
	"context"
	"log/slog"

	"github.com/smrobot988-design/Agora/pkg/core"
)

// SupervisorStrategizer uses a "supervisor" Runner (typically an LLM agent)
// to dynamically coordinate sub-runners. The supervisor agent's tool registry
// contains AgentTool entries for each sub-runner. The LLM naturally decides
// which sub-agent to invoke and when, using the existing tool-calling mechanism.
//
// This pattern requires no changes to core.Agent, core.Router, or core.Loop —
// it works entirely through tool composition.
//
// Setup:
//  1. Create sub-agents as Runners.
//  2. Wrap each as an AgentTool and register in the supervisor's tool.Registry.
//  3. Create the supervisor Agent with that registry.
//  4. Wrap the supervisor Agent as a Runner and pass it here.
type SupervisorStrategizer struct {
	supervisor Runner
	maxRounds  int // safety limit for supervisor turns (0 = use agent default)
}

// SupervisorOption configures a SupervisorStrategizer.
type SupervisorOption func(*SupervisorStrategizer)

// WithSupervisorMaxRounds sets a maximum number of rounds for the supervisor.
func WithSupervisorMaxRounds(n int) SupervisorOption {
	return func(s *SupervisorStrategizer) { s.maxRounds = n }
}

// NewSupervisorStrategizer creates a supervisor strategy.
// The supervisor Runner should be an Agent whose tool registry includes
// AgentTool entries for each sub-runner it can delegate to.
func NewSupervisorStrategizer(supervisor Runner, opts ...SupervisorOption) *SupervisorStrategizer {
	s := &SupervisorStrategizer{supervisor: supervisor}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Orchestrate delegates entirely to the supervisor agent.
// The runners parameter is ignored because the supervisor already has
// access to sub-agents via AgentTool in its tool registry.
// It is included in the call to satisfy the Strategizer interface.
func (s *SupervisorStrategizer) Orchestrate(ctx context.Context, input string, _ []Runner) (*core.Result, error) {
	slog.Info("supervisor delegating",
		"supervisor", s.supervisor.Name(),
		"input_length", len(input),
	)

	result, err := s.supervisor.Run(ctx, input)
	if err != nil {
		return nil, err
	}

	slog.Info("supervisor completed",
		"supervisor", s.supervisor.Name(),
		"input_tokens", result.TotalInputTokens,
		"output_tokens", result.TotalOutputTokens,
		"turns", result.Turns,
	)

	return result, nil
}
