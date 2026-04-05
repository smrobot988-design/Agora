package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/smrobot988-design/Agora/pkg/core"
)

// DebateFormatter constructs the input for each runner in a debate round.
// It injects the original question and other runners' responses from the
// previous round so each runner can refine its position.
type DebateFormatter interface {
	// Format builds the input for a runner in the given round.
	//   - round: current round (0-based).
	//   - runnerName: the name of the runner receiving this input.
	//   - originalInput: the original user question.
	//   - previousResponses: map of runner name → response text from the previous round.
	Format(round int, runnerName string, originalInput string, previousResponses map[string]string) string
}

// DefaultDebateFormatter is a built-in formatter that lists other runners'
// responses and asks the current runner to refine its position.
type DefaultDebateFormatter struct{}

// Format constructs a prompt with the original question and other experts' opinions.
func (f *DefaultDebateFormatter) Format(round int, runnerName string, originalInput string, previousResponses map[string]string) string {
	if len(previousResponses) == 0 {
		return originalInput
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Original question: %s\n\n", originalInput))
	b.WriteString("Other experts' opinions from the previous round:\n")

	// Sort keys for deterministic output.
	names := make([]string, 0, len(previousResponses))
	for name := range previousResponses {
		if name != runnerName {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	for _, name := range names {
		b.WriteString(fmt.Sprintf("- %s: %s\n", name, previousResponses[name]))
	}

	b.WriteString(fmt.Sprintf("\nBased on the above opinions, please refine your position as %s.", runnerName))
	return b.String()
}

// DebateStrategizer runs multiple rounds of debate between runners.
// In each round, every runner sees the previous round's outputs from all
// runners. After maxRounds, a Merger synthesizes the final answer.
type DebateStrategizer struct {
	maxRounds int
	merger    Merger
	formatter DebateFormatter
}

// DebateOption configures a DebateStrategizer.
type DebateOption func(*DebateStrategizer)

// WithMaxRounds sets the number of debate rounds.
func WithMaxRounds(n int) DebateOption {
	return func(s *DebateStrategizer) { s.maxRounds = n }
}

// WithDebateFormatter sets the formatter used to construct round inputs.
func WithDebateFormatter(f DebateFormatter) DebateOption {
	return func(s *DebateStrategizer) { s.formatter = f }
}

// NewDebateStrategizer creates a debate strategy.
//
//   - maxRounds: number of debate rounds (minimum 1).
//   - merger: used to synthesize the final answer from all responses.
//   - opts: optional configuration (formatter, etc.).
func NewDebateStrategizer(maxRounds int, merger Merger, opts ...DebateOption) *DebateStrategizer {
	if maxRounds < 1 {
		maxRounds = 1
	}
	s := &DebateStrategizer{
		maxRounds: maxRounds,
		merger:    merger,
		formatter: &DefaultDebateFormatter{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Orchestrate runs the debate and merges the final results.
func (s *DebateStrategizer) Orchestrate(ctx context.Context, input string, runners []Runner) (*core.Result, error) {
	if len(runners) == 0 {
		return &core.Result{}, nil
	}

	// previousResponses tracks each runner's last response.
	previousResponses := make(map[string]string)
	var allResults []*core.Result

	for round := 0; round < s.maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("debate round %d: context cancelled: %w", round, err)
		}

		slog.Info("debate round starting",
			"round", round+1,
			"total_rounds", s.maxRounds,
			"runners", len(runners),
		)

		roundResponses := make(map[string]string)

		for _, runner := range runners {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("debate round %d runner %s: context cancelled: %w", round, runner.Name(), err)
			}

			// First round: use original input directly.
			// Subsequent rounds: format with previous responses.
			var roundInput string
			if round == 0 {
				roundInput = input
			} else {
				roundInput = s.formatter.Format(round, runner.Name(), input, previousResponses)
			}

			result, err := runner.Run(ctx, roundInput)
			if err != nil {
				return nil, fmt.Errorf("debate round %d runner %s: %w", round, runner.Name(), err)
			}

			roundResponses[runner.Name()] = result.Text
			allResults = append(allResults, result)

			slog.Debug("debate response",
				"round", round+1,
				"runner", runner.Name(),
				"response_length", len(result.Text),
			)
		}

		// Update previous responses for next round.
		previousResponses = roundResponses
	}

	// Collect only the final round's results for merging.
	finalRoundResults := allResults[len(allResults)-len(runners):]

	slog.Info("debate merging final results",
		"total_rounds", s.maxRounds,
		"results_to_merge", len(finalRoundResults),
	)

	merged, err := s.merger.Merge(finalRoundResults)
	if err != nil {
		return nil, fmt.Errorf("debate merge: %w", err)
	}

	// Add token usage from all rounds (not just the final merge).
	for _, r := range allResults[:len(allResults)-len(runners)] {
		merged.TotalInputTokens += r.TotalInputTokens
		merged.TotalOutputTokens += r.TotalOutputTokens
	}

	return merged, nil
}
