package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/smrobot988-design/Agora/pkg/core"
	"golang.org/x/sync/errgroup"
)

// ParallelStrategizer runs all Runners concurrently using errgroup
// and merges the results with the provided Merger.
type ParallelStrategizer struct {
	merger     Merger
	maxWorkers int // 0 = unlimited concurrency
}

// ParallelOption configures a ParallelStrategizer.
type ParallelOption func(*ParallelStrategizer)

// WithMaxWorkers limits the number of concurrent runners.
func WithMaxWorkers(n int) ParallelOption {
	return func(s *ParallelStrategizer) { s.maxWorkers = n }
}

// NewParallelStrategizer creates a parallel strategy with the given merger.
func NewParallelStrategizer(merger Merger, opts ...ParallelOption) *ParallelStrategizer {
	s := &ParallelStrategizer{merger: merger}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Orchestrate runs all runners concurrently and merges results.
func (s *ParallelStrategizer) Orchestrate(ctx context.Context, input string, runners []Runner) (*core.Result, error) {
	if len(runners) == 0 {
		return &core.Result{}, nil
	}

	g, gctx := errgroup.WithContext(ctx)
	if s.maxWorkers > 0 {
		g.SetLimit(s.maxWorkers)
	}

	results := make([]*core.Result, len(runners))

	for i, runner := range runners {
		slog.Info("parallel runner starting", "runner", runner.Name())

		g.Go(func() error {
			result, err := runner.Run(gctx, input)
			if err != nil {
				return fmt.Errorf("parallel runner %s: %w", runner.Name(), err)
			}

			slog.Info("parallel runner completed",
				"runner", runner.Name(),
				"input_tokens", result.TotalInputTokens,
				"output_tokens", result.TotalOutputTokens,
			)

			results[i] = result
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return s.merger.Merge(ctx, results)
}
