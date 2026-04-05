package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/smrobot988-design/Agora/pkg/core"
)

// Transformer modifies text between pipeline stages.
// This is an optional hook for reformatting or enriching the
// output of one stage before passing it as input to the next.
type Transformer interface {
	Transform(stageIndex int, runnerName string, result *core.Result) string
}

// PipelineStrategizer runs Runners sequentially, feeding the output
// of each stage as the input to the next. The final stage's Result
// is returned with aggregated token counts from all stages.
type PipelineStrategizer struct {
	transformer Transformer
}

// PipelineOption configures a PipelineStrategizer.
type PipelineOption func(*PipelineStrategizer)

// WithTransformer sets a Transformer to modify text between stages.
func WithTransformer(t Transformer) PipelineOption {
	return func(s *PipelineStrategizer) { s.transformer = t }
}

// NewPipelineStrategizer creates a pipeline strategy.
func NewPipelineStrategizer(opts ...PipelineOption) *PipelineStrategizer {
	s := &PipelineStrategizer{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Orchestrate runs each runner in sequence.
func (s *PipelineStrategizer) Orchestrate(ctx context.Context, input string, runners []Runner) (*core.Result, error) {
	if len(runners) == 0 {
		return &core.Result{}, nil
	}

	current := input
	aggregated := &core.Result{}

	for i, runner := range runners {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("pipeline stage %d (%s): context cancelled: %w", i, runner.Name(), err)
		}

		slog.Info("pipeline stage starting",
			"stage", i,
			"runner", runner.Name(),
		)

		result, err := runner.Run(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("pipeline stage %d (%s): %w", i, runner.Name(), err)
		}

		// Aggregate token usage across all stages.
		aggregated.TotalInputTokens += result.TotalInputTokens
		aggregated.TotalOutputTokens += result.TotalOutputTokens
		aggregated.Turns += result.Turns

		slog.Info("pipeline stage completed",
			"stage", i,
			"runner", runner.Name(),
			"input_tokens", result.TotalInputTokens,
			"output_tokens", result.TotalOutputTokens,
		)

		// Determine input for next stage.
		if s.transformer != nil {
			current = s.transformer.Transform(i, runner.Name(), result)
		} else {
			current = result.Text
		}

		// Last stage: use its text as the final output.
		if i == len(runners)-1 {
			aggregated.Text = result.Text
		}
	}

	return aggregated, nil
}
