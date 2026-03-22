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
func (a *Agent) runLoop(ctx context.Context, result *Result) (*Result, error) {
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

		// Call the LLM.
		slog.Info("calling LLM",
			"turn", turn,
			"provider", a.provider.Name(),
			"messages", len(msgs),
		)

		resp, err := a.provider.Chat(ctx, llm.ChatParams{
			System:   a.memory.SystemPrompt(),
			Messages: msgs,
			Tools:    a.registry.Definitions(),
		})
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
			toolResults, err := a.executeTools(ctx, decision.ToolCalls)
			if err != nil {
				return nil, fmt.Errorf("execute tools (turn %d): %w", turn, err)
			}

			// Log tool results before feeding back to LLM.
			for _, tr := range toolResults {
				isError := ""
				if tr.IsError {
					isError = " [error]"
				}
				// Truncate long output for log readability.
				content := tr.Content
				if len(content) > 200 {
					content = content[:200] + "... [truncated]"
				}
				slog.Info("tool result" + isError,
					"turn", turn,
					"call_id", tr.ToolCallID,
					"output", content,
				)
			}

			if err := a.memory.AddToolResult(toolResults); err != nil {
				return nil, fmt.Errorf("add tool results: %w", err)
			}
			// Continue loop: LLM will see the tool results next iteration.

		case ActionError:
			return nil, fmt.Errorf("routing error (turn %d): %w", turn, decision.Error)
		}
	}

	return nil, fmt.Errorf("max turns (%d) exceeded", a.maxTurns)
}

// executeTools runs all tool calls and returns the results.
// Tool execution errors are captured as error tool_results, not as fatal errors.
// The only fatal error from this method is context cancellation.
func (a *Agent) executeTools(ctx context.Context, calls []ValidatedToolCall) ([]llm.ToolResult, error) {
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

		output, err := vc.Tool.Execute(ctx, vc.Call.Input)
		if err != nil {
			// Tool execution error: inject as error tool_result.
			// This lets the LLM see the error and potentially recover.
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
