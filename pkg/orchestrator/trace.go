package orchestrator

import (
	"fmt"

	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/core/trace"
)

// OrchestratorTracer records cross-agent traces.
// It creates a top-level trace for the entire orchestration run,
// and spans for each runner invocation. Sub-agent internal spans
// (from their core.Tracer) can be merged in via MergeSubTrace.
type OrchestratorTracer struct {
	trace    *trace.Trace
	exporter trace.Exporter
}

// NewOrchestratorTracer creates a tracer for an orchestration run.
// If exporter is nil, defaults to JSONExporter.
func NewOrchestratorTracer(name string, exporter trace.Exporter) *OrchestratorTracer {
	if exporter == nil {
		exporter = &trace.JSONExporter{}
	}
	return &OrchestratorTracer{
		trace:    trace.NewTrace(name),
		exporter: exporter,
	}
}

// activeOrchestratorSpan records an in-progress orchestration-level span.
type activeOrchestratorSpan struct {
	tracer *OrchestratorTracer
	span   *trace.Span
}

// StartOrchestrationSpan begins a span for the entire orchestration run.
func (t *OrchestratorTracer) StartOrchestrationSpan(name string) *activeOrchestratorSpan {
	span := t.trace.StartSpan(trace.SpanKindAgent, name, "")
	return &activeOrchestratorSpan{tracer: t, span: span}
}

// StartRunnerSpan begins a span for a single runner invocation.
// It is a child of parentID (typically the orchestration span's ID).
func (t *OrchestratorTracer) StartRunnerSpan(runnerName, parentID string) *activeOrchestratorSpan {
	span := t.trace.StartSpan(trace.SpanKindAgent, runnerName, parentID)
	return &activeOrchestratorSpan{tracer: t, span: span}
}

// End finalizes the span with the result and error.
func (s *activeOrchestratorSpan) End(result *core.Result, err error) {
	if err != nil {
		s.span.Error = err.Error()
	}
	if result != nil {
		s.span.InputTokens = result.TotalInputTokens
		s.span.OutputTokens = result.TotalOutputTokens
	}
	s.tracer.trace.EndSpan(s.span)
}

// SpanID returns the span ID, useful for establishing parent relationships.
func (s *activeOrchestratorSpan) SpanID() string {
	return s.span.SpanID
}

// Flush exports the trace via the configured exporter.
func (t *OrchestratorTracer) Flush() error {
	return t.exporter.Export(t.trace)
}

// TraceID returns the global trace ID for correlation with sub-agent traces.
func (t *OrchestratorTracer) TraceID() string {
	return t.trace.TraceID
}

// MergeSubTrace merges a sub-agent's trace spans into this orchestrator trace.
// All spans' TraceID is updated to the orchestrator's TraceID.
// If a span has no parent, it is attached to parentSpanID.
func (t *OrchestratorTracer) MergeSubTrace(subTrace *trace.Trace, parentSpanID string) {
	if subTrace == nil {
		return
	}
	for _, span := range subTrace.Spans {
		// Re-parent if the span has no existing parent.
		if span.ParentID == "" {
			span.ParentID = parentSpanID
		}
		// Normalize TraceID.
		span.TraceID = t.trace.TraceID
		t.trace.Spans = append(t.trace.Spans, span)
	}
}

// MergeResultSnapshot records the final result into the trace.
func (t *OrchestratorTracer) MergeResultSnapshot(result *core.Result) {
	if result == nil {
		return
	}
	t.trace.Finalize(&trace.ResultSnapshot{
		Text:              result.Text,
		TotalInputTokens:  result.TotalInputTokens,
		TotalOutputTokens: result.TotalOutputTokens,
		Turns:             result.Turns,
	})
}

// Error records an error into the current span.
func (s *activeOrchestratorSpan) Error(err error) {
	s.span.Error = err.Error()
}

// SetInputTokens records token usage.
func (s *activeOrchestratorSpan) SetInputTokens(tokens int) {
	s.span.InputTokens = tokens
}

// SetOutputTokens records token usage.
func (s *activeOrchestratorSpan) SetOutputTokens(tokens int) {
	s.span.OutputTokens = tokens
}

// String implements fmt.Stringer for convenient logging.
func (s *activeOrchestratorSpan) String() string {
	return fmt.Sprintf("span %s (%s)", s.span.SpanID, s.span.Name)
}
