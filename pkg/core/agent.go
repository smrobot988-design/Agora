package core

import (
	"context"
	"fmt"
	"time"

	"github.com/smrobot988-design/Agora/pkg/core/trace"
	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

// defaultMaxTurns is the safety limit to prevent infinite loops.
const defaultMaxTurns = 10

// Agent ties together a Provider, Memory, tool Registry, and Router
// to execute agentic loops.
type Agent struct {
	provider       llm.Provider
	memory         *Memory
	registry       *tool.Registry
	router         *Router
	maxTurns       int
	tracer         *Tracer
	loopDetector   *LoopDetector
	summarizer     *Summarizer
	streamCallback func(*llm.PartialResponse)
	lastToolResults []llm.ToolResult
}

// AgentOption configures an Agent.
type AgentOption func(*Agent)

// WithMaxTurns sets the maximum number of LLM call iterations.
func WithMaxTurns(n int) AgentOption {
	return func(a *Agent) { a.maxTurns = n }
}

// WithTracer enables trace recording with the given exporter (nil = JSONExporter).
func WithTracer(exporter trace.Exporter) AgentOption {
	return func(a *Agent) {
		a.tracer = NewTracer(a.provider.Name(), exporter)
	}
}

// WithLoopDetector enables loop detection with the default threshold.
func WithLoopDetector() AgentOption {
	return func(a *Agent) {
		a.loopDetector = NewLoopDetector()
	}
}

// WithSummarizer enables structured run summary logging.
func WithSummarizer() AgentOption {
	return func(a *Agent) {
		a.summarizer = NewSummarizer()
	}
}

// WithStreamCallback enables streaming of LLM output via the provided callback.
// The callback is invoked for each partial response event during LLM calls.
// This is useful for CLI applications that want to display tokens in real-time.
func WithStreamCallback(cb func(*llm.PartialResponse)) AgentOption {
	return func(a *Agent) {
		a.streamCallback = cb
	}
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
	summary := &RunSummary{StartTime: time.Now()}
	a.lastToolResults = nil

	finalResult, runErr := a.runLoop(ctx, result, summary)

	// Capture the last tool results from memory for orchestration access.
	a.lastToolResults, _ = a.memory.LastToolResults()

	if a.tracer != nil {
		a.tracer.FinalizeSnapshot(finalResult)
		if err := a.tracer.Flush(); err != nil {
			// Flush failure is non-fatal; log and continue.
			_ = err
		}
	}
	if a.summarizer != nil {
		summary.totalInputTokens = finalResult.TotalInputTokens
		summary.totalOutputTokens = finalResult.TotalOutputTokens
		a.summarizer.Log(ctx, summary)
	}

	return finalResult, runErr
}

// LastToolResults returns the tool results from the most recent Agent.Run call.
// Used by orchestrator patterns (e.g., Swarm handoff detection) to inspect
// what tools were called without relying on LLM text parsing.
func (a *Agent) LastToolResults() ([]llm.ToolResult, error) {
	return a.lastToolResults, nil
}
