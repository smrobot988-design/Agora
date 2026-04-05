package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/smrobot988-design/Agora/pkg/core"
)

// Merger combines multiple Results into one.
// Used by Parallel, Debate, and other patterns that produce multiple outputs.
// Context is always propagated through Merge to support cancellation and deadlines
// across the full orchestration chain.
type Merger interface {
	Merge(ctx context.Context, results []*core.Result) (*core.Result, error)
}

// ConcatMerger concatenates all result texts with a separator
// and sums token counts across all results.
type ConcatMerger struct {
	Separator string
}

// Merge concatenates texts and aggregates token usage.
func (m *ConcatMerger) Merge(_ context.Context, results []*core.Result) (*core.Result, error) {
	if len(results) == 0 {
		return &core.Result{}, nil
	}

	texts := make([]string, 0, len(results))
	merged := &core.Result{}

	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Text != "" {
			texts = append(texts, r.Text)
		}
		merged.TotalInputTokens += r.TotalInputTokens
		merged.TotalOutputTokens += r.TotalOutputTokens
		if r.Turns > merged.Turns {
			merged.Turns = r.Turns
		}
	}

	sep := m.Separator
	if sep == "" {
		sep = "\n"
	}
	merged.Text = strings.Join(texts, sep)
	return merged, nil
}

// FirstSuccessMerger returns the first non-nil result.
// Useful for race/failover patterns where only one result is needed.
type FirstSuccessMerger struct{}

// Merge returns the first non-nil result.
func (m *FirstSuccessMerger) Merge(_ context.Context, results []*core.Result) (*core.Result, error) {
	for _, r := range results {
		if r != nil {
			return r, nil
		}
	}
	return nil, fmt.Errorf("no successful results to merge")
}

// LLMMerger uses a Runner (typically an LLM agent) to synthesize
// multiple results into a single coherent answer. This is the most
// powerful merger, suitable for debate consensus and complex aggregation.
type LLMMerger struct {
	// Runner is the agent that performs the synthesis.
	Runner Runner

	// PromptTemplate is a format string with two verbs:
	// %d for the number of results, %s for the concatenated texts.
	// Example: "Given %d expert opinions:\n%s\nSynthesize a final answer."
	PromptTemplate string
}

// Merge invokes the Runner with a synthesized prompt containing all results.
// Context is propagated to allow cancellation of the synthesis step.
func (m *LLMMerger) Merge(ctx context.Context, results []*core.Result) (*core.Result, error) {
	if len(results) == 0 {
		return &core.Result{}, nil
	}

	// Build the combined text for the synthesis prompt.
	var parts []string
	totalInput, totalOutput, maxTurns := 0, 0, 0
	for _, r := range results {
		if r == nil {
			continue
		}
		parts = append(parts, r.Text)
		totalInput += r.TotalInputTokens
		totalOutput += r.TotalOutputTokens
		if r.Turns > maxTurns {
			maxTurns = r.Turns
		}
	}

	combined := strings.Join(parts, "\n\n---\n\n")
	prompt := fmt.Sprintf(m.PromptTemplate, len(parts), combined)

	synthResult, err := m.Runner.Run(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM merger synthesis: %w", err)
	}

	// Aggregate token usage from all inputs plus the synthesis run.
	synthResult.TotalInputTokens += totalInput
	synthResult.TotalOutputTokens += totalOutput
	synthResult.Turns += maxTurns
	return synthResult, nil
}
