package core

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/smrobot988-design/Agora/pkg/llm"
)

// runLoop is the core agent loop. It repeatedly calls the LLM, routes the
// response, and executes tools until a final answer is produced or the
// maximum number of turns is reached.
func (a *Agent) runLoop(ctx context.Context, result *Result, summary *RunSummary) (*Result, error) {
	for turn := 1; turn <= a.maxTurns; turn++ {
		// Check context cancellation before each LLM call.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled: %w", err)
		}

		// Build messages from memory.
		msgs, err := a.memory.Messages()
		if err != nil {
			return nil, fmt.Errorf("get messages: %w", err)
		}

		slog.Info("calling LLM",
			"turn", turn,
			"provider", a.provider.Name(),
			"messages", len(msgs),
		)

		// Trace the LLM call. Span must be ended immediately after Chat,
		// regardless of success or error, to avoid span leaks.
		var llmSpan *activeSpan
		var resp *llm.Response
		if a.tracer != nil {
			llmSpan = a.tracer.StartLLMSpan()
		}

		resp, err = a.provider.Chat(ctx, llm.ChatParams{
			System:   a.memory.SystemPrompt(),
			Messages: msgs,
			Tools:    a.registry.Definitions(),
		})
		// End span before error check: error is recorded inside EndLLM.
		if a.tracer != nil && llmSpan != nil {
			llmSpan.EndLLM(resp, err)
		}
		if err != nil {
			return nil, fmt.Errorf("provider chat (turn %d): %w", turn, err)
		}

		// Log LLM response details.
		slog.Info("LLM response",
			"turn", turn,
			"stop_reason", resp.StopReason,
			"input_tokens", resp.InputTokens,
			"output_tokens", resp.OutputTokens,
		)
		if resp.Text != "" {
			slog.Info("LLM text output",
				"turn", turn,
				"text", resp.Text,
			)
		}
		if len(resp.ToolCalls) > 0 {
			slog.Info("LLM requested tools",
				"turn", turn,
				"tool_count", len(resp.ToolCalls),
			)
			for _, tc := range resp.ToolCalls {
				slog.Info("tool call",
					"turn", turn,
					"tool", tc.Name,
					"call_id", tc.ID,
					"input", tc.Input,
				)
			}
		}

		// Accumulate token usage.
		result.TotalInputTokens += resp.InputTokens
		result.TotalOutputTokens += resp.OutputTokens
		result.Turns = turn

		// Store assistant response in memory.
		if err := a.memory.AddAssistantResponse(resp); err != nil {
			return nil, fmt.Errorf("add assistant response: %w", err)
		}

		// Route the response.
		decision := a.router.Route(resp)

		switch decision.Action {
		case ActionFinal:
			result.Text = decision.Text
			return result, nil

		case ActionToolCall:
			toolResults, err := a.executeTools(ctx, decision.ToolCalls, summary)
			if err != nil {
				return nil, fmt.Errorf("execute tools (turn %d): %w", turn, err)
			}

			// Log tool results before feeding back to LLM.
			for _, tr := range toolResults {
				isError := ""
				if tr.IsError {
					isError = " [error]"
				}
				content := tr.Content
				if len(content) > 200 {
					content = content[:200] + "... [truncated]"
				}
				slog.Info("tool result"+isError,
					"turn", turn,
					"call_id", tr.ToolCallID,
					"output", content,
				)
			}

			if err := a.memory.AddToolResult(toolResults); err != nil {
				return nil, fmt.Errorf("add tool results: %w", err)
			}

			// Detect loops before feeding results back to LLM.
			if a.loopDetector != nil {
				loopResult := a.loopDetector.Detect()
				if loopResult.Type != LoopTypeNone {
					summary.LoopsDetected++
					slog.Warn("loop detected, terminating",
						"turn", turn,
						"type", loopResult.Type,
						"detail", loopResult.Detail,
					)
					return nil, fmt.Errorf("loop detected (turn %d): %s", loopResult.Turn, loopResult.Detail)
				}
				a.loopDetector.Record(turn, decision.ToolCalls, toolResults)
			}

		case ActionError:
			return nil, fmt.Errorf("routing error (turn %d): %w", turn, decision.Error)
		}
	}

	return nil, fmt.Errorf("max turns (%d) exceeded", a.maxTurns)
}

// executeTools runs all tool calls and returns the results.
// Tool execution errors are captured as error tool_results, not as fatal errors.
// The only fatal error from this method is context cancellation.
func (a *Agent) executeTools(ctx context.Context, calls []ValidatedToolCall, summary *RunSummary) ([]llm.ToolResult, error) {
	results := make([]llm.ToolResult, 0, len(calls))

	for _, vc := range calls {
		// Check context before each tool execution.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled before tool %q: %w", vc.Call.Name, err)
		}

		slog.Info("executing tool",
			"tool", vc.Call.Name,
			"call_id", vc.Call.ID,
		)

		// Trace tool execution. EndTool is called first to record the result
		// (including error), then we handle the error separately.
		var toolSpan *activeSpan
		var output string
		var err error

		if a.tracer != nil {
			toolSpan = a.tracer.StartToolSpan(vc.Call.Name, vc.Call.Input)
		}

		output, err = vc.Tool.Execute(ctx, vc.Call.Input)

		// End span before error check: EndTool records error into the span if any.
		if a.tracer != nil && toolSpan != nil {
			toolSpan.EndTool(output, err)
		}
		if err != nil {
			slog.Warn("tool execution error",
				"tool", vc.Call.Name,
				"error", err,
			)
			results = append(results, llm.ToolResult{
				ToolCallID: vc.Call.ID,
				Content:    fmt.Sprintf("Error: %s", err.Error()),
				IsError:    true,
			})
			continue
		}

		results = append(results, llm.ToolResult{
			ToolCallID: vc.Call.ID,
			Content:    output,
		})
	}

	return results, nil
}
