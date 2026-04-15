package core

import (
	"github.com/smrobot988-design/Agora/pkg/core/trace"
	"github.com/smrobot988-design/Agora/pkg/llm"
)

// Tracer records spans for LLM calls and tool executions.
type Tracer struct {
	trace    *trace.Trace
	exporter trace.Exporter
	rootSpan *trace.Span
}

// NewTracer creates a Tracer for a new run. exporter may be nil (defaults to JSONExporter).
func NewTracer(providerName string, exporter trace.Exporter) *Tracer {
	if exporter == nil {
		exporter = &trace.JSONExporter{}
	}
	return &Tracer{
		trace:    trace.NewTrace(providerName),
		exporter: exporter,
	}
}

// GetTrace returns the underlying trace.
func (t *Tracer) GetTrace() *trace.Trace {
	return t.trace
}

// Flush exports the trace using the stored exporter.
func (t *Tracer) Flush() error {
	return t.exporter.Export(t.trace)
}

// activeSpan records an in-progress span. Returned by StartLLMSpan / StartToolSpan.
type activeSpan struct {
	tracer *Tracer
	span   *trace.Span
}

// StartLLMSpan begins an LLM span.
func (t *Tracer) StartLLMSpan() *activeSpan {
	parentID := ""
	if t.rootSpan != nil {
		parentID = t.rootSpan.SpanID
	}
	span := t.trace.StartSpan(trace.SpanKindLLM, "Chat", parentID)
	return &activeSpan{tracer: t, span: span}
}

// EndLLM populates the LLM span with the response and finalizes it.
func (s *activeSpan) EndLLM(resp *llm.Response, err error) {
	if err != nil {
		s.span.Error = err.Error()
	} else {
		s.span.StopReason = string(resp.StopReason)
		s.span.InputTokens = resp.InputTokens
		s.span.OutputTokens = resp.OutputTokens
	}
	s.tracer.trace.EndSpan(s.span)
}

// StartToolSpan begins a tool execution span as a child of the current LLM span.
func (t *Tracer) StartToolSpan(toolName string, input map[string]interface{}) *activeSpan {
	span := t.trace.StartSpan(trace.SpanKindTool, toolName, "")
	span.ToolName = toolName
	span.ToolInput = input
	return &activeSpan{tracer: t, span: span}
}

// EndTool finalizes the tool span with the output or error.
func (s *activeSpan) EndTool(output string, err error) {
	if err != nil {
		s.span.ToolError = err.Error()
	} else {
		s.span.ToolOutput = output
	}
	s.tracer.trace.EndSpan(s.span)
}

// FinalizeSnapshot records the final result snapshot into the trace.
func (t *Tracer) FinalizeSnapshot(result *Result) {
	t.trace.Finalize(&trace.ResultSnapshot{
		Text:              result.Text,
		ReasoningText:     result.ReasoningText,
		TotalInputTokens:  result.TotalInputTokens,
		TotalOutputTokens: result.TotalOutputTokens,
		Turns:             result.Turns,
	})
}
