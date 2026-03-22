package core

import (
	"context"
	"log/slog"
	"time"
)

// RunSummary records a summary of a single Agent.Run() call.
type RunSummary struct {
	StartTime         time.Time
	Turns             []TurnSummary
	LoopsDetected     int
	totalInputTokens  int
	totalOutputTokens int
}

// TurnSummary is the per-turn summary.
type TurnSummary struct {
	Turn          int
	LLMStopReason string
	ToolsCalled   []string
	ToolErrors    []string
	InputTokens   int
	OutputTokens  int
}

// Summarizer logs structured run summaries via slog.
type Summarizer struct {
	logger *slog.Logger
	level  slog.Level
}

// NewSummarizer creates a Summarizer using the default slog logger at Info level.
func NewSummarizer() *Summarizer {
	return &Summarizer{
		logger: slog.Default(),
		level:  slog.LevelInfo,
	}
}

// WithLevel sets the minimum slog level for summaries.
func (s *Summarizer) WithLevel(level slog.Level) *Summarizer {
	s.level = level
	return s
}

// Log prints the run summary as structured log entries.
// ctx is passed from the caller (typically Agent.Run); it must not be context.Background().
func (s *Summarizer) Log(ctx context.Context, summary *RunSummary) {
	// Log the summary line at the configured level.
	if s.level <= slog.LevelInfo {
		s.logger.Log(ctx, s.level, "Run complete",
			"turns", len(summary.Turns),
			"input_tokens", summary.totalInputTokens,
			"output_tokens", summary.totalOutputTokens,
			"total_tokens", summary.totalInputTokens+summary.totalOutputTokens,
			"loops_detected", summary.LoopsDetected,
			"duration_ms", float64(time.Since(summary.StartTime).Microseconds())/1000.0,
		)
	}

	// Per-turn details at debug level only.
	if s.level <= slog.LevelDebug {
		for _, turn := range summary.Turns {
			s.logger.Log(ctx, slog.LevelInfo, "Turn summary",
				"turn", turn.Turn,
				"llm_stop_reason", turn.LLMStopReason,
				"tools_called", turn.ToolsCalled,
				"tool_errors", turn.ToolErrors,
				"input_tokens", turn.InputTokens,
				"output_tokens", turn.OutputTokens,
			)
		}
	}
}
