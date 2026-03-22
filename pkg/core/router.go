package core

import (
	"fmt"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

// Action represents the kind of next step the agent should take.
type Action string

const (
	// ActionFinal means the LLM produced a final text response.
	ActionFinal Action = "final"

	// ActionToolCall means the LLM requested one or more tool calls.
	ActionToolCall Action = "tool_call"

	// ActionError means the response could not be routed.
	ActionError Action = "error"
)

// Decision is the output of the Router: what the agent loop should do next.
type Decision struct {
	// Action is the type of decision.
	Action Action

	// Text is the final text response (only set when Action == ActionFinal).
	Text string

	// ToolCalls lists the validated tool calls (only set when Action == ActionToolCall).
	ToolCalls []ValidatedToolCall

	// Error describes the routing failure (only set when Action == ActionError).
	Error error
}

// ValidatedToolCall pairs a schema.ToolCall with its resolved Tool implementation.
type ValidatedToolCall struct {
	Call schema.ToolCall
	Tool tool.Tool
}

// Router examines an LLM response and determines the next action.
type Router struct {
	registry *tool.Registry
}

// NewRouter creates a Router backed by the given tool registry.
func NewRouter(registry *tool.Registry) *Router {
	return &Router{registry: registry}
}

// Route inspects the response and returns a Decision.
func (r *Router) Route(resp *llm.Response) Decision {
	switch resp.StopReason {
	case llm.StopReasonEndTurn:
		return Decision{Action: ActionFinal, Text: resp.Text}

	case llm.StopReasonToolUse:
		return r.routeToolCalls(resp.ToolCalls)

	case llm.StopReasonMaxTokens:
		return Decision{
			Action: ActionError,
			Error:  fmt.Errorf("response truncated: max_tokens reached"),
		}

	default:
		return Decision{
			Action: ActionError,
			Error:  fmt.Errorf("unknown stop reason: %s", resp.StopReason),
		}
	}
}

// routeToolCalls validates each tool call against the registry.
func (r *Router) routeToolCalls(calls []schema.ToolCall) Decision {
	if len(calls) == 0 {
		return Decision{
			Action: ActionError,
			Error:  fmt.Errorf("stop_reason is tool_use but no tool calls present"),
		}
	}

	validated := make([]ValidatedToolCall, 0, len(calls))
	for _, call := range calls {
		t, ok := r.registry.Get(call.Name)
		if !ok {
			return Decision{
				Action: ActionError,
				Error:  fmt.Errorf("unknown tool: %q", call.Name),
			}
		}
		validated = append(validated, ValidatedToolCall{Call: call, Tool: t})
	}
	return Decision{Action: ActionToolCall, ToolCalls: validated}
}
