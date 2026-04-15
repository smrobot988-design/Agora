package trace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SpanKind classifies what a span represents.
type SpanKind string

const (
	SpanKindLLM   SpanKind = "llm"
	SpanKindTool  SpanKind = "tool"
	SpanKindAgent SpanKind = "agent"
)

// Span represents a single timed operation within a trace.
type Span struct {
	SpanID       string                 `json:"span_id"`
	ParentID     string                 `json:"parent_id,omitempty"`
	TraceID      string                 `json:"trace_id"`
	Kind         SpanKind               `json:"kind"`
	Name         string                 `json:"name"`
	StartTime    time.Time              `json:"start_time"`
	EndTime      time.Time              `json:"end_time"`
	DurationMS   float64                `json:"duration_ms"`
	StopReason   string                 `json:"stop_reason,omitempty"`
	InputTokens  int                    `json:"input_tokens,omitempty"`
	OutputTokens int                    `json:"output_tokens,omitempty"`
	ToolName     string                 `json:"tool_name,omitempty"`
	ToolInput    map[string]interface{} `json:"tool_input,omitempty"`
	ToolOutput   string                 `json:"tool_output,omitempty"`
	ToolError    string                 `json:"tool_error,omitempty"`
	Error        string                 `json:"error,omitempty"`
}

// ResultSnapshot captures the final result of an agent run.
type ResultSnapshot struct {
	Text              string `json:"text"`
	ReasoningText     string `json:"reasoning_text,omitempty"`
	TotalInputTokens  int    `json:"total_input_tokens"`
	TotalOutputTokens int    `json:"total_output_tokens"`
	Turns             int    `json:"turns"`
}

// Trace is the complete record of an agent run.
type Trace struct {
	TraceID    string          `json:"trace_id"`
	AgentName  string          `json:"agent_name"`
	StartTime  time.Time       `json:"start_time"`
	EndTime    time.Time       `json:"end_time"`
	DurationMS float64         `json:"duration_ms"`
	Spans      []Span          `json:"spans"`
	Result     *ResultSnapshot `json:"result,omitempty"`
}

// MarshalJSON serializes the trace to JSON.
func (t *Trace) MarshalJSON() ([]byte, error) {
	type Alias Trace
	return json.Marshal(struct {
		Alias
		DurationMS float64 `json:"duration_ms"`
	}{
		Alias:      Alias(*t),
		DurationMS: t.DurationMS,
	})
}

// newID generates a unique 16-character hex string.
// Not cryptographically secure but sufficient for trace IDs.
func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewTrace creates a new trace with a unique ID.
func NewTrace(agentName string) *Trace {
	return &Trace{
		TraceID:   newID(),
		AgentName: agentName,
		StartTime: time.Now(),
		Spans:     []Span{},
	}
}

// StartSpan begins a new span as a child of parentID.
func (t *Trace) StartSpan(kind SpanKind, name, parentID string) *Span {
	return &Span{
		SpanID:    newID(),
		ParentID:  parentID,
		TraceID:   t.TraceID,
		Kind:      kind,
		Name:      name,
		StartTime: time.Now(),
	}
}

// EndSpan finalizes a span and appends it to the trace.
func (t *Trace) EndSpan(span *Span) {
	span.EndTime = time.Now()
	span.DurationMS = float64(span.EndTime.Sub(span.StartTime).Microseconds()) / 1000.0
	t.Spans = append(t.Spans, *span)
}

// Finalize completes the trace with the result and total duration.
func (t *Trace) Finalize(result *ResultSnapshot) {
	t.EndTime = time.Now()
	t.DurationMS = float64(t.EndTime.Sub(t.StartTime).Microseconds()) / 1000.0
	t.Result = result
}

// Exporter writes a Trace to a storage backend.
type Exporter interface {
	Export(t *Trace) error
}

// JSONExporter writes a Trace to a JSON file.
type JSONExporter struct {
	Dir string // Directory to write files. Defaults to current directory.
}

// Export writes the trace as trace-<traceID>.json to the configured directory.
func (e *JSONExporter) Export(t *Trace) error {
	filename := "trace-" + t.TraceID + ".json"
	dir := e.Dir
	if dir == "" {
		dir = "."
	}
	path := filepath.Join(dir, filename)

	data, err := t.MarshalJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// NoOpExporter discards all traces (used for testing or when tracing is disabled).
type NoOpExporter struct{}

// Export does nothing and returns nil.
func (e *NoOpExporter) Export(t *Trace) error { return nil }
