package core

import (
	"context"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

func newTestRegistry(t *testing.T) *tool.Registry {
	t.Helper()
	r := tool.NewRegistry()
	_ = r.Register(&tool.Func{
		Def: schema.ToolDefinition{Name: "get_weather", Description: "Get weather"},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			return `{"temp":"20°C"}`, nil
		},
	})
	_ = r.Register(&tool.Func{
		Def: schema.ToolDefinition{Name: "read_file", Description: "Read a file"},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			return "file contents", nil
		},
	})
	return r
}

func TestRouteEndTurn(t *testing.T) {
	router := NewRouter(newTestRegistry(t))
	resp := &llm.Response{StopReason: llm.StopReasonEndTurn, Text: "Hello!"}
	d := router.Route(resp)
	if d.Action != ActionFinal {
		t.Fatalf("expected ActionFinal, got %s", d.Action)
	}
	if d.Text != "Hello!" {
		t.Fatalf("expected 'Hello!', got %q", d.Text)
	}
}

func TestRouteToolUse(t *testing.T) {
	router := NewRouter(newTestRegistry(t))
	resp := &llm.Response{
		StopReason: llm.StopReasonToolUse,
		ToolCalls: []schema.ToolCall{
			{ID: "c1", Name: "get_weather", Input: map[string]interface{}{"city": "Tokyo"}},
		},
	}
	d := router.Route(resp)
	if d.Action != ActionToolCall {
		t.Fatalf("expected ActionToolCall, got %s", d.Action)
	}
	if len(d.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(d.ToolCalls))
	}
	if d.ToolCalls[0].Call.Name != "get_weather" {
		t.Fatalf("expected get_weather, got %s", d.ToolCalls[0].Call.Name)
	}
	if d.ToolCalls[0].Tool == nil {
		t.Fatal("expected resolved tool, got nil")
	}
}

func TestRouteToolUseUnknownTool(t *testing.T) {
	router := NewRouter(newTestRegistry(t))
	resp := &llm.Response{
		StopReason: llm.StopReasonToolUse,
		ToolCalls: []schema.ToolCall{
			{ID: "c1", Name: "nonexistent"},
		},
	}
	d := router.Route(resp)
	if d.Action != ActionError {
		t.Fatalf("expected ActionError, got %s", d.Action)
	}
	if d.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestRouteMaxTokens(t *testing.T) {
	router := NewRouter(newTestRegistry(t))
	resp := &llm.Response{StopReason: llm.StopReasonMaxTokens}
	d := router.Route(resp)
	if d.Action != ActionError {
		t.Fatalf("expected ActionError, got %s", d.Action)
	}
}

func TestRouteUnknownStopReason(t *testing.T) {
	router := NewRouter(newTestRegistry(t))
	resp := &llm.Response{StopReason: "unknown_reason"}
	d := router.Route(resp)
	if d.Action != ActionError {
		t.Fatalf("expected ActionError, got %s", d.Action)
	}
}

func TestRouteToolUseEmptyCalls(t *testing.T) {
	router := NewRouter(newTestRegistry(t))
	resp := &llm.Response{
		StopReason: llm.StopReasonToolUse,
		ToolCalls:  []schema.ToolCall{},
	}
	d := router.Route(resp)
	if d.Action != ActionError {
		t.Fatalf("expected ActionError, got %s", d.Action)
	}
}

func TestRouteMultipleToolCalls(t *testing.T) {
	router := NewRouter(newTestRegistry(t))
	resp := &llm.Response{
		StopReason: llm.StopReasonToolUse,
		ToolCalls: []schema.ToolCall{
			{ID: "c1", Name: "get_weather"},
			{ID: "c2", Name: "read_file"},
		},
	}
	d := router.Route(resp)
	if d.Action != ActionToolCall {
		t.Fatalf("expected ActionToolCall, got %s", d.Action)
	}
	if len(d.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(d.ToolCalls))
	}
}
