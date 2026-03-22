package core

import (
	"context"
	"fmt"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

// defaultMaxTurns is the safety limit to prevent infinite loops.
const defaultMaxTurns = 10

// Agent ties together a Provider, Memory, tool Registry, and Router
// to execute agentic loops.
type Agent struct {
	provider llm.Provider
	memory   *Memory
	registry *tool.Registry
	router   *Router
	maxTurns int
}

// AgentOption configures an Agent.
type AgentOption func(*Agent)

// WithMaxTurns sets the maximum number of LLM call iterations.
func WithMaxTurns(n int) AgentOption {
	return func(a *Agent) { a.maxTurns = n }
}

// NewAgent creates an Agent with the given provider, memory, registry, and options.
func NewAgent(provider llm.Provider, mem *Memory, registry *tool.Registry, opts ...AgentOption) *Agent {
	a := &Agent{
		provider: provider,
		memory:   mem,
		registry: registry,
		router:   NewRouter(registry),
		maxTurns: defaultMaxTurns,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Run executes a single agent task: sends the user input through the LLM,
// handles tool calls in a loop, and returns when the LLM produces a final
// response or the maximum number of turns is reached.
func (a *Agent) Run(ctx context.Context, input string) (*Result, error) {
	if err := a.memory.AddUserMessage(input); err != nil {
		return nil, fmt.Errorf("add user message: %w", err)
	}

	result := &Result{}
	return a.runLoop(ctx, result)
}
