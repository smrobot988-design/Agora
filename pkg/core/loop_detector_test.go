package core

import (
	"testing"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

// makeToolCall creates a ValidatedToolCall for testing.
func makeToolCall(id, name string, input map[string]interface{}) ValidatedToolCall {
	return ValidatedToolCall{
		Call: schema.ToolCall{ID: id, Name: name, Input: input},
	}
}

// makeToolResult creates a ToolResult for testing.
func makeToolResult(id, content string, isError bool) llm.ToolResult {
	return llm.ToolResult{ToolCallID: id, Content: content, IsError: isError}
}

func TestLoopDetectorSameToolSameInput(t *testing.T) {
	detector := NewLoopDetector().WithSameToolThreshold(3)

	calls := []ValidatedToolCall{
		makeToolCall("c1", "read_file", map[string]interface{}{"path": "/a.txt"}),
	}
	results := []llm.ToolResult{makeToolResult("c1", "content", false)}

	// Record same call 3 times.
	for turn := 1; turn <= 3; turn++ {
		detector.Record(turn, calls, results)
	}

	result := detector.Detect()
	if result.Type != LoopTypeSameToolSameInput {
		t.Fatalf("expected LoopTypeSameToolSameInput, got %v", result.Type)
	}
}

func TestLoopDetectorDifferentCallsNoLoop(t *testing.T) {
	detector := NewLoopDetector().WithSameToolThreshold(3)

	calls1 := []ValidatedToolCall{
		makeToolCall("c1", "read_file", map[string]interface{}{"path": "/a.txt"}),
	}
	calls2 := []ValidatedToolCall{
		makeToolCall("c2", "read_file", map[string]interface{}{"path": "/b.txt"}),
	}
	results := []llm.ToolResult{makeToolResult("c1", "content", false)}

	detector.Record(1, calls1, results)
	detector.Record(2, calls2, results)
	detector.Record(3, calls1, results)

	result := detector.Detect()
	if result.Type != LoopTypeNone {
		t.Fatalf("expected LoopTypeNone, got %v", result.Type)
	}
}

func TestLoopDetectorSameError(t *testing.T) {
	detector := NewLoopDetector()

	calls := []ValidatedToolCall{makeToolCall("c1", "read_file", map[string]interface{}{"path": "/a.txt"})}

	errResult := []llm.ToolResult{makeToolResult("c1", "Error: permission denied", true)}

	detector.Record(1, calls, errResult)
	detector.Record(2, calls, errResult) // same error repeated

	result := detector.Detect()
	if result.Type != LoopTypeSameError {
		t.Fatalf("expected LoopTypeSameError, got %v", result.Type)
	}
}

func TestLoopDetectorDifferentErrorsNoLoop(t *testing.T) {
	detector := NewLoopDetector()

	calls := []ValidatedToolCall{makeToolCall("c1", "read_file", map[string]interface{}{"path": "/a.txt"})}

	err1 := []llm.ToolResult{makeToolResult("c1", "Error: permission denied", true)}
	err2 := []llm.ToolResult{makeToolResult("c1", "Error: not found", true)}

	detector.Record(1, calls, err1)
	detector.Record(2, calls, err2)

	result := detector.Detect()
	if result.Type != LoopTypeNone {
		t.Fatalf("expected LoopTypeNone, got %v", result.Type)
	}
}

func TestLoopDetectorEmpty(t *testing.T) {
	detector := NewLoopDetector()
	result := detector.Detect()
	if result.Type != LoopTypeNone {
		t.Fatalf("expected LoopTypeNone on empty history, got %v", result.Type)
	}
}

func TestLoopDetectorInsufficientHistory(t *testing.T) {
	detector := NewLoopDetector().WithSameToolThreshold(3)

	calls := []ValidatedToolCall{
		makeToolCall("c1", "read_file", map[string]interface{}{"path": "/a.txt"}),
	}
	results := []llm.ToolResult{makeToolResult("c1", "content", false)}

	// Record only 2 times, threshold is 3.
	detector.Record(1, calls, results)
	detector.Record(2, calls, results)

	result := detector.Detect()
	if result.Type != LoopTypeNone {
		t.Fatalf("expected LoopTypeNone with insufficient history, got %v", result.Type)
	}
}

func TestLoopDetectorMixedToolAndError(t *testing.T) {
	detector := NewLoopDetector()

	calls := []ValidatedToolCall{makeToolCall("c1", "read_file", map[string]interface{}{"path": "/a.txt"})}

	errResult := []llm.ToolResult{makeToolResult("c1", "Error: oops", true)}
	okResult := []llm.ToolResult{makeToolResult("c1", "ok", false)}

	// ok, error, error → same error loop
	detector.Record(1, calls, okResult)
	detector.Record(2, calls, errResult)
	detector.Record(3, calls, errResult)

	result := detector.Detect()
	if result.Type != LoopTypeSameError {
		t.Fatalf("expected LoopTypeSameError, got %v", result.Type)
	}
}

func TestLoopDetectorThreshold2(t *testing.T) {
	detector := NewLoopDetector().WithSameToolThreshold(2)

	calls := []ValidatedToolCall{
		makeToolCall("c1", "echo", map[string]interface{}{"msg": "hi"}),
	}
	results := []llm.ToolResult{makeToolResult("c1", "hi", false)}

	detector.Record(1, calls, results)
	detector.Record(2, calls, results) // 2nd same call = loop with threshold=2

	result := detector.Detect()
	if result.Type != LoopTypeSameToolSameInput {
		t.Fatalf("expected LoopTypeSameToolSameInput with threshold=2, got %v", result.Type)
	}
}

func TestLoopDetectorNoFalsePositiveDifferentInputs(t *testing.T) {
	detector := NewLoopDetector().WithSameToolThreshold(3)

	calls1 := []ValidatedToolCall{
		makeToolCall("c1", "read_file", map[string]interface{}{"path": "/a.txt"}),
	}
	calls2 := []ValidatedToolCall{
		makeToolCall("c1", "read_file", map[string]interface{}{"path": "/a.txt", "offset": 1}), // different input
	}
	results := []llm.ToolResult{makeToolResult("c1", "content", false)}

	detector.Record(1, calls1, results)
	detector.Record(2, calls1, results)
	detector.Record(3, calls2, results) // different fingerprint

	result := detector.Detect()
	if result.Type != LoopTypeNone {
		t.Fatalf("expected LoopTypeNone for different inputs, got %v", result.Type)
	}
}

func TestLoopDetectorBoundedHistory(t *testing.T) {
	detector := NewLoopDetector().WithSameToolThreshold(3)

	calls := []ValidatedToolCall{
		makeToolCall("c1", "echo", map[string]interface{}{"msg": "x"}),
	}
	results := []llm.ToolResult{makeToolResult("c1", "x", false)}

	// Record 25 rounds. History is bounded to 20.
	for i := 1; i <= 25; i++ {
		detector.Record(i, calls, results)
	}

	result := detector.Detect()
	// With bounded history (max 20), last 3 are all the same, so should detect.
	if result.Type != LoopTypeSameToolSameInput {
		t.Fatalf("expected LoopTypeSameToolSameInput after bounded history, got %v", result.Type)
	}
}
