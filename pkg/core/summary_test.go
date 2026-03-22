package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSummarizerLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	sum := &Summarizer{logger: logger, level: slog.LevelInfo}
	summary := &RunSummary{
		StartTime:         time.Now().Add(-1 * time.Second),
		Turns:            []TurnSummary{{Turn: 1, LLMStopReason: "tool_use", ToolsCalled: []string{"read_file"}, ToolErrors: nil, InputTokens: 100, OutputTokens: 50}},
		LoopsDetected:    0,
		totalInputTokens:  100,
		totalOutputTokens: 50,
	}

	sum.Log(context.Background(), summary)

	output := buf.String()
	if !strings.Contains(output, "Run complete") {
		t.Fatalf("expected 'Run complete' in output, got: %s", output)
	}
	if !strings.Contains(output, `"turns":1`) && !strings.Contains(output, `"turns": 1`) {
		t.Fatalf("expected turns=1 in output, got: %s", output)
	}
}

func TestSummarizerDebugTurnDetails(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	// Summarizer level is Debug: turn details are logged because level <= Debug.
	sum := &Summarizer{logger: logger, level: slog.LevelDebug}
	summary := &RunSummary{
		StartTime:          time.Now(),
		Turns:              []TurnSummary{{Turn: 1, LLMStopReason: "tool_use", ToolsCalled: []string{"echo"}, ToolErrors: []string{"error"}, InputTokens: 10, OutputTokens: 5}},
		LoopsDetected:      1,
		totalInputTokens:   10,
		totalOutputTokens:  5,
	}

	sum.Log(context.Background(), summary)

	output := buf.String()
	if !strings.Contains(output, "Turn summary") {
		t.Fatalf("expected 'Turn summary' in debug output, got: %s", output)
	}
	if !strings.Contains(output, "echo") {
		t.Fatalf("expected tool name 'echo' in output, got: %s", output)
	}
}

func TestSummarizerEmptyTurns(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	sum := &Summarizer{logger: logger, level: slog.LevelInfo}
	summary := &RunSummary{
		StartTime:          time.Now(),
		Turns:              []TurnSummary{},
		LoopsDetected:      0,
		totalInputTokens:   0,
		totalOutputTokens:  0,
	}

	sum.Log(context.Background(), summary)

	output := buf.String()
	if !strings.Contains(output, "Run complete") {
		t.Fatalf("expected 'Run complete' in output, got: %s", output)
	}
}

func TestSummarizerMultipleTurns(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	sum := &Summarizer{logger: logger, level: slog.LevelInfo}
	summary := &RunSummary{
		StartTime:         time.Now(),
		Turns: []TurnSummary{
			{Turn: 1, LLMStopReason: "tool_use", ToolsCalled: []string{"read_file"}, InputTokens: 100, OutputTokens: 50},
			{Turn: 2, LLMStopReason: "end_turn", InputTokens: 200, OutputTokens: 80},
		},
		LoopsDetected:     0,
		totalInputTokens:  300,
		totalOutputTokens: 130,
	}

	sum.Log(context.Background(), summary)

	output := buf.String()
	var entries []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSON line: %s", line)
		}
		entries = append(entries, m)
	}

	if len(entries) < 1 {
		t.Fatal("expected at least 1 log entry")
	}
	// First entry should be "Run complete".
	msg, ok := entries[0]["msg"].(string)
	if !ok || msg != "Run complete" {
		t.Fatalf("first entry should be 'Run complete', got: %v", entries[0])
	}
}

func TestSummarizerWithLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	// Set level to Warn so Info messages are filtered.
	sum := &Summarizer{logger: logger, level: slog.LevelWarn}
	summary := &RunSummary{
		StartTime:          time.Now(),
		Turns:              []TurnSummary{},
		LoopsDetected:      0,
		totalInputTokens:   0,
		totalOutputTokens:  0,
	}

	sum.Log(context.Background(), summary)

	// With Warn level, the Info message should not appear.
	output := buf.String()
	if strings.Contains(output, "Run complete") {
		t.Fatalf("expected 'Run complete' NOT to appear at Warn level, got: %s", output)
	}
}

func TestSummarizerLoopsDetected(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	sum := &Summarizer{logger: logger, level: slog.LevelInfo}
	summary := &RunSummary{
		StartTime:          time.Now(),
		Turns:              []TurnSummary{},
		LoopsDetected:      2,
		totalInputTokens:   500,
		totalOutputTokens:  200,
	}

	sum.Log(context.Background(), summary)

	output := buf.String()
	if !strings.Contains(output, "loops_detected") && !strings.Contains(output, "loops_detected") {
		// Check for the field in JSON output
		var entry map[string]interface{}
		lines := strings.Split(strings.TrimSpace(output), "\n")
		if len(lines) > 0 {
			json.Unmarshal([]byte(lines[0]), &entry)
			if loops, ok := entry["loops_detected"].(float64); !ok || int(loops) != 2 {
				t.Fatalf("expected loops_detected=2, got: %v", entry["loops_detected"])
			}
		}
	}
}
