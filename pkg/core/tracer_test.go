package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core/trace"
	"github.com/smrobot988-design/Agora/pkg/llm"
)

func TestTracerCreate(t *testing.T) {
	tracer := NewTracer("claude", nil)
	if tracer == nil {
		t.Fatal("NewTracer returned nil")
	}
	if tracer.trace == nil {
		t.Fatal("trace is nil")
	}
	if tracer.trace.AgentName != "claude" {
		t.Fatalf("expected agent name 'claude', got %q", tracer.trace.AgentName)
	}
	if tracer.trace.TraceID == "" {
		t.Fatal("trace ID should not be empty")
	}
}

func TestTracerLLMSpan(t *testing.T) {
	tracer := NewTracer("claude", nil)

	span := tracer.StartLLMSpan()
	if span == nil {
		t.Fatal("StartLLMSpan returned nil")
	}
	if span.span == nil {
		t.Fatal("span is nil")
	}
	if span.span.Kind != trace.SpanKindLLM {
		t.Fatalf("expected SpanKindLLM, got %v", span.span.Kind)
	}

	// Simulate LLM response.
	resp := &llm.Response{
		StopReason:   llm.StopReasonToolUse,
		InputTokens:  100,
		OutputTokens: 50,
	}
	span.EndLLM(resp, nil)

	// Span should be recorded in trace.
	if len(tracer.trace.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(tracer.trace.Spans))
	}
	recorded := tracer.trace.Spans[0]
	if recorded.StopReason != "tool_use" {
		t.Fatalf("expected stop_reason 'tool_use', got %q", recorded.StopReason)
	}
	if recorded.InputTokens != 100 {
		t.Fatalf("expected input_tokens 100, got %d", recorded.InputTokens)
	}
	if recorded.OutputTokens != 50 {
		t.Fatalf("expected output_tokens 50, got %d", recorded.OutputTokens)
	}
}

func TestTracerLLMSpanError(t *testing.T) {
	tracer := NewTracer("claude", nil)

	span := tracer.StartLLMSpan()
	span.EndLLM(nil, &mockError{"connection reset"})

	if len(tracer.trace.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(tracer.trace.Spans))
	}
	if tracer.trace.Spans[0].Error == "" {
		t.Fatal("expected error to be recorded in span")
	}
}

func TestTracerToolSpan(t *testing.T) {
	tracer := NewTracer("claude", nil)

	span := tracer.StartToolSpan("read_file", map[string]interface{}{"path": "/a.txt"})
	if span.span.ToolName != "read_file" {
		t.Fatalf("expected tool name 'read_file', got %q", span.span.ToolName)
	}
	if span.span.Kind != trace.SpanKindTool {
		t.Fatalf("expected SpanKindTool, got %v", span.span.Kind)
	}

	span.EndTool("file content", nil)

	if len(tracer.trace.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(tracer.trace.Spans))
	}
	recorded := tracer.trace.Spans[0]
	if recorded.ToolOutput != "file content" {
		t.Fatalf("expected tool output 'file content', got %q", recorded.ToolOutput)
	}
}

func TestTracerToolSpanError(t *testing.T) {
	tracer := NewTracer("claude", nil)

	span := tracer.StartToolSpan("read_file", map[string]interface{}{"path": "/a.txt"})
	span.EndTool("", &mockError{"permission denied"})

	if len(tracer.trace.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(tracer.trace.Spans))
	}
	if tracer.trace.Spans[0].ToolError == "" {
		t.Fatal("expected tool error to be recorded")
	}
}

func TestTracerFinalizeSnapshot(t *testing.T) {
	tracer := NewTracer("claude", nil)

	span := tracer.StartLLMSpan()
	span.EndLLM(&llm.Response{StopReason: llm.StopReasonEndTurn, InputTokens: 10, OutputTokens: 5}, nil)

	result := &Result{
		Text:              "final answer",
		ReasoningText:     "reasoning",
		TotalInputTokens:  100,
		TotalOutputTokens: 50,
		Turns:             3,
	}
	tracer.FinalizeSnapshot(result)

	if tracer.trace.Result == nil {
		t.Fatal("result snapshot should not be nil")
	}
	if tracer.trace.Result.Text != "final answer" {
		t.Fatalf("expected text 'final answer', got %q", tracer.trace.Result.Text)
	}
	if tracer.trace.Result.ReasoningText != "reasoning" {
		t.Fatalf("expected reasoning text, got %q", tracer.trace.Result.ReasoningText)
	}
	if tracer.trace.Result.TotalInputTokens != 100 {
		t.Fatalf("expected 100 input tokens, got %d", tracer.trace.Result.TotalInputTokens)
	}
	if tracer.trace.Result.Turns != 3 {
		t.Fatalf("expected 3 turns, got %d", tracer.trace.Result.Turns)
	}
}

func TestTracerFlushJSON(t *testing.T) {
	tmpDir := t.TempDir()

	tracer := NewTracer("claude", &trace.JSONExporter{Dir: tmpDir})
	span := tracer.StartLLMSpan()
	span.EndLLM(&llm.Response{StopReason: llm.StopReasonEndTurn, InputTokens: 10, OutputTokens: 5}, nil)
	tracer.FinalizeSnapshot(&Result{Text: "ok", TotalInputTokens: 10, TotalOutputTokens: 5, Turns: 1})

	err := tracer.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Check file was created.
	files, err := filepath.Glob(filepath.Join(tmpDir, "trace-*.json"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 trace file, got %d", len(files))
	}

	// Verify JSON is valid.
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var traceJSON map[string]interface{}
	if err := json.Unmarshal(data, &traceJSON); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	if traceJSON["agent_name"] != "claude" {
		t.Fatalf("expected agent_name 'claude', got %v", traceJSON["agent_name"])
	}
}

func TestTracerNoOpExporter(t *testing.T) {
	tracer := NewTracer("claude", &trace.NoOpExporter{})
	span := tracer.StartLLMSpan()
	span.EndLLM(&llm.Response{StopReason: llm.StopReasonEndTurn, InputTokens: 10, OutputTokens: 5}, nil)

	err := tracer.Flush()
	if err != nil {
		t.Fatalf("Flush with NoOpExporter failed: %v", err)
	}
}

func TestTracerDurationMS(t *testing.T) {
	tracer := NewTracer("claude", nil)

	span := tracer.StartLLMSpan()
	span.EndLLM(&llm.Response{StopReason: llm.StopReasonEndTurn, InputTokens: 10, OutputTokens: 5}, nil)

	if tracer.trace.Spans[0].DurationMS < 0 {
		t.Fatalf("duration_ms should be non-negative, got %v", tracer.trace.Spans[0].DurationMS)
	}
}

func TestTracerMultipleSpans(t *testing.T) {
	tracer := NewTracer("claude", nil)

	// LLM call.
	llmSpan := tracer.StartLLMSpan()
	llmSpan.EndLLM(&llm.Response{StopReason: llm.StopReasonToolUse, InputTokens: 10, OutputTokens: 5}, nil)

	// Tool call.
	toolSpan := tracer.StartToolSpan("echo", map[string]interface{}{"msg": "hello"})
	toolSpan.EndTool("hello", nil)

	// Another LLM call.
	llmSpan2 := tracer.StartLLMSpan()
	llmSpan2.EndLLM(&llm.Response{StopReason: llm.StopReasonEndTurn, InputTokens: 20, OutputTokens: 10}, nil)

	if len(tracer.trace.Spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(tracer.trace.Spans))
	}
}

func TestTracerTraceIDConsistent(t *testing.T) {
	tracer := NewTracer("claude", nil)
	id1 := tracer.trace.TraceID

	span := tracer.StartLLMSpan()
	span.EndLLM(&llm.Response{StopReason: llm.StopReasonEndTurn, InputTokens: 10, OutputTokens: 5}, nil)

	span2 := tracer.StartLLMSpan()
	span2.EndLLM(&llm.Response{StopReason: llm.StopReasonEndTurn, InputTokens: 10, OutputTokens: 5}, nil)

	if tracer.trace.TraceID != id1 {
		t.Fatalf("trace ID changed: was %s, now %s", id1, tracer.trace.TraceID)
	}

	for _, s := range tracer.trace.Spans {
		if s.TraceID != id1 {
			t.Fatalf("span trace_id %s != trace.trace_id %s", s.TraceID, id1)
		}
	}
}

func TestTracerEndTimeSet(t *testing.T) {
	tracer := NewTracer("claude", nil)

	span := tracer.StartLLMSpan()
	span.EndLLM(&llm.Response{StopReason: llm.StopReasonEndTurn, InputTokens: 10, OutputTokens: 5}, nil)
	tracer.FinalizeSnapshot(&Result{Text: "ok", TotalInputTokens: 10, TotalOutputTokens: 5, Turns: 1})

	if tracer.trace.EndTime.IsZero() {
		t.Fatal("trace end time should be set after finalize")
	}
}

// mockError is a simple error implementation for testing.
type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }
