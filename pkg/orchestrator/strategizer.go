package orchestrator

import (
	"context"
	"log/slog"

	"github.com/smrobot988-design/Agora/pkg/core"
)

// Strategizer defines how to orchestrate a set of Runners for a given input.
// Each orchestration pattern (Pipeline, Parallel, Supervisor, Debate, etc.)
// is a concrete Strategizer implementation.
//
// To add a new orchestration pattern, implement this single-method interface.
type Strategizer interface {
	Orchestrate(ctx context.Context, input string, runners []Runner) (*core.Result, error)
}

// Orchestrator pairs a Strategizer with a set of Runners.
// It implements Runner itself, enabling recursive composition:
// an Orchestrator can be used as a Runner inside another Orchestrator.
type Orchestrator struct {
	name     string
	strategy Strategizer
	runners  []Runner
	tracer   *OrchestratorTracer
}

// OrchestratorOption configures an Orchestrator.
type OrchestratorOption func(*Orchestrator)

// WithOrchestratorTracer enables cross-agent tracing.
func WithOrchestratorTracer(t *OrchestratorTracer) OrchestratorOption {
	return func(o *Orchestrator) { o.tracer = t }
}

// NewOrchestrator creates an Orchestrator with the given strategy and runners.
func NewOrchestrator(name string, strategy Strategizer, runners []Runner, opts ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{
		name:     name,
		strategy: strategy,
		runners:  runners,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Run delegates to the strategy. Orchestrator satisfies the Runner interface.
func (o *Orchestrator) Run(ctx context.Context, input string) (*core.Result, error) {
	slog.Info("orchestrator starting",
		"name", o.name,
		"strategy", strategyName(o.strategy),
		"runners", runnerNames(o.runners),
	)

	var orchSpan *activeOrchestratorSpan
	if o.tracer != nil {
		orchSpan = o.tracer.StartOrchestrationSpan(o.name)
	}

	result, err := o.strategy.Orchestrate(ctx, input, o.runners)

	if o.tracer != nil && orchSpan != nil {
		orchSpan.End(result, err)
	}

	if err != nil {
		slog.Error("orchestrator failed", "name", o.name, "error", err)
	} else {
		slog.Info("orchestrator completed",
			"name", o.name,
			"input_tokens", result.TotalInputTokens,
			"output_tokens", result.TotalOutputTokens,
		)
	}

	if o.tracer != nil {
		if flushErr := o.tracer.Flush(); flushErr != nil {
			slog.Warn("tracer flush failed", "error", flushErr)
		}
	}

	return result, err
}

// Name returns the orchestrator's name.
func (o *Orchestrator) Name() string { return o.name }

// Tracer returns the orchestrator's tracer (may be nil).
func (o *Orchestrator) Tracer() *OrchestratorTracer { return o.tracer }

// strategyName returns a short name for logging.
func strategyName(s Strategizer) string {
	switch s.(type) {
	case *PipelineStrategizer:
		return "pipeline"
	case *ParallelStrategizer:
		return "parallel"
	case *SupervisorStrategizer:
		return "supervisor"
	case *DebateStrategizer:
		return "debate"
	default:
		return "custom"
	}
}

// runnerNames extracts names from a slice of Runners.
func runnerNames(runners []Runner) []string {
	names := make([]string, len(runners))
	for i, r := range runners {
		names[i] = r.Name()
	}
	return names
}
