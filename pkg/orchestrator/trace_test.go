package orchestrator

import (
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/core/trace"
)

func TestOrchestratorTracerNew(t *testing.T) {
	tracer := NewOrchestratorTracer("test", nil)
	if tracer.TraceID() == "" {
		t.Error("expected non-empty trace ID")
	}
}

func TestOrchestratorTracerSpanLifecycle(t *testing.T) {
	tracer := NewOrchestratorTracer("test", nil)

	// Start an orchestration span.
	orchSpan := tracer.StartOrchestrationSpan("article-pipeline")
	if orchSpan.SpanID() == "" {
		t.Error("expected non-empty span ID")
	}
	if orchSpan.span.Kind != trace.SpanKindAgent {
		t.Errorf("kind = %v, want SpanKindAgent", orchSpan.span.Kind)
	}

	// Start a runner span as child.
	runnerSpan := tracer.StartRunnerSpan("researcher", orchSpan.SpanID())
	if runnerSpan.SpanID() == "" {
		t.Error("expected non-empty runner span ID")
	}
	if runnerSpan.span.ParentID != orchSpan.SpanID() {
		t.Errorf("parentID = %q, want %q", runnerSpan.span.ParentID, orchSpan.SpanID())
	}

	// End spans.
	runnerSpan.End(&core.Result{
		Text:              "research output",
		TotalInputTokens:  100,
		TotalOutputTokens: 50,
	}, nil)
	orchSpan.End(&core.Result{Text: "final"}, nil)
}

func TestOrchestratorTracerMergeSubTrace(t *testing.T) {
	tracer := NewOrchestratorTracer("test", nil)
	orchSpan := tracer.StartOrchestrationSpan("pipeline")

	// Simulate a sub-agent's trace with a span that has no parent
	// (representing the top-level llm span from a single-agent run).
	subTrace := trace.NewTrace("researcher")
	subLLMSpan := subTrace.StartSpan(trace.SpanKindLLM, "Chat", "")
	subLLMSpan.EndTime = subLLMSpan.StartTime
	subLLMSpan.DurationMS = 500
	subTrace.EndSpan(subLLMSpan)

	// Start runner span and end it BEFORE merging (avoids slice-reallocation stale pointer).
	runnerSpan := tracer.StartRunnerSpan("researcher", orchSpan.SpanID())
	runnerSpanID := runnerSpan.SpanID()
	runnerSpan.End(nil, nil)

	// Merge: sub spans without parent should be re-parented to the runner span.
	// This appends to Spans, potentially reallocating — but runnerSpan is already ended.
	tracer.MergeSubTrace(subTrace, runnerSpanID)

	orchSpan.End(nil, nil)

	// Verify: 3 spans total (orch + runner + merged llm).
	if len(tracer.trace.Spans) != 3 {
		t.Errorf("spans = %d, want 3", len(tracer.trace.Spans))
	}

	// All spans should have the same TraceID.
	for _, span := range tracer.trace.Spans {
		if span.TraceID != tracer.TraceID() {
			t.Errorf("span TraceID = %q, want %q", span.TraceID, tracer.TraceID())
		}
	}

	// Merged llm span should be re-parented to runner (since it had no parent).
	var mergedLLM trace.Span
	for _, span := range tracer.trace.Spans {
		if span.Name == "Chat" && span.Kind == trace.SpanKindLLM {
			mergedLLM = span
			break
		}
	}
	if mergedLLM.ParentID != runnerSpanID {
		t.Errorf("merged llm parentID = %q, want %q", mergedLLM.ParentID, runnerSpanID)
	}
}

func TestOrchestratorTracerMergeSubTraceNil(t *testing.T) {
	tracer := NewOrchestratorTracer("test", nil)
	tracer.MergeSubTrace(nil, "some-parent") // Should not panic.
	if len(tracer.trace.Spans) != 0 {
		t.Errorf("spans = %d, want 0", len(tracer.trace.Spans))
	}
}

func TestOrchestratorTracerMergeResultSnapshot(t *testing.T) {
	tracer := NewOrchestratorTracer("test", nil)
	tracer.MergeResultSnapshot(&core.Result{
		Text:              "final answer",
		TotalInputTokens:  500,
		TotalOutputTokens: 300,
		Turns:             5,
	})
	if tracer.trace.Result == nil {
		t.Fatal("expected result snapshot")
	}
	if tracer.trace.Result.Text != "final answer" {
		t.Errorf("text = %q, want %q", tracer.trace.Result.Text, "final answer")
	}
}

func TestOrchestratorTracerMergeResultSnapshotNil(t *testing.T) {
	tracer := NewOrchestratorTracer("test", nil)
	tracer.MergeResultSnapshot(nil) // Should not panic.
	if tracer.trace.Result != nil {
		t.Error("expected nil result")
	}
}

func TestActiveSpanString(t *testing.T) {
	tracer := NewOrchestratorTracer("test", nil)
	span := tracer.StartOrchestrationSpan("pipeline")
	s := span.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}
