package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

// TurnContext carries all data for a single agent turn, passed through pipeline stages.
type TurnContext struct {
	// loop
	turn   int
	isDone bool // set by route stage when ActionFinal or ActionError
	isLoop bool // set by executeTools stage when loop detected

	// message
	messages []llm.Message

	// tool call
	decision    *Decision
	toolCalls   []ValidatedToolCall
	toolResults []llm.ToolResult

	// resp
	response *llm.Response
	result   *Result
	summary  *RunSummary
	err      error
}

func newTurnContext(turn int, result *Result, summary *RunSummary) *TurnContext {
	return &TurnContext{
		turn:    turn,
		result:  result,
		summary: summary,
	}
}

// PipelineStage is a function that processes one turn and returns an updated context.
type PipelineStage func(*TurnContext) *TurnContext

// runLoop is the core agent loop. It repeatedly calls the LLM, routes the
// response, and executes tools until a final answer is produced or the
// maximum number of turns is reached.
func (a *Agent) runLoop(ctx context.Context, result *Result, summary *RunSummary) (*Result, error) {
	slog.Info("calling LLm run loop start")
	for turn := 1; turn <= a.maxTurns; turn++ {
		tc := newTurnContext(turn, result, summary)

		tc = a.prepareTurn(ctx, tc)
		if tc.err != nil {
			return nil, tc.err
		}

		tc = a.callLLM(ctx, tc)
		if tc.err != nil {
			return nil, tc.err
		}

		tc = a.logResponse(tc)
		tc = a.storeMemory(tc)

		tc = a.route(tc)
		if tc.isDone {
			return tc.result, tc.err
		}
		tc = a.executeTools(ctx, tc)
		if tc.isLoop {
			return nil, tc.err
		}
	}

	return nil, fmt.Errorf("max turns (%d) exceeded", a.maxTurns)
}

// prepareTurn checks context cancellation and builds messages from memory.
func (a *Agent) prepareTurn(ctx context.Context, tc *TurnContext) *TurnContext {
	if err := ctx.Err(); err != nil {
		tc.err = fmt.Errorf("context cancelled: %w", err)
		return tc
	}

	msgs, err := a.memory.Messages()
	if err != nil {
		tc.err = fmt.Errorf("get messages: %w", err)
		return tc
	}
	tc.messages = msgs

	slog.Info("calling LLM",
		"turn", tc.turn,
		"provider", a.provider.Name(),
		"messages", len(msgs),
	)

	return tc
}

// callLLM invokes the LLM (streaming or batch) and returns a complete *llm.Response.
func (a *Agent) callLLM(ctx context.Context, tc *TurnContext) *TurnContext {
	var llmSpan *activeSpan
	if a.tracer != nil {
		llmSpan = a.tracer.StartLLMSpan()
	}

	params := llm.ChatParams{
		System:   a.memory.SystemPrompt(),
		Messages: tc.messages,
		Tools:    a.registry.Definitions(),
	}

	var resp *llm.Response
	if a.streamCallback != nil {
		resp, tc.err = a.streamingChat(ctx, params)
	} else {
		resp, tc.err = a.provider.Chat(ctx, params)
	}

	// End span before error check: EndLLM records error into the span.
	if a.tracer != nil && llmSpan != nil {
		llmSpan.EndLLM(resp, tc.err)
	}
	if tc.err != nil {
		tc.err = fmt.Errorf("provider chat (turn %d): %w", tc.turn, tc.err)
		return tc
	}

	tc.response = resp
	return tc
}

// streamingChat calls ChatStream and accumulates events into a complete *llm.Response.
func (a *Agent) streamingChat(ctx context.Context, params llm.ChatParams) (*llm.Response, error) {
	accum := newStreamAccumulator()
	cb := func(event *llm.PartialResponse) {
		accum.onEvent(event)
		a.streamCallback(event)
	}

	err := a.provider.ChatStream(ctx, params, cb)
	if err == llm.ErrStreamingUnsupported {
		return a.provider.Chat(ctx, params)
	}
	if err != nil {
		return nil, err
	}
	return accum.toResponse(), nil
}

// logResponse logs the LLM response details.
func (a *Agent) logResponse(tc *TurnContext) *TurnContext {
	resp := tc.response

	slog.Info("LLM response",
		"turn", tc.turn,
		"stop_reason", resp.StopReason,
		"input_tokens", resp.InputTokens,
		"output_tokens", resp.OutputTokens,
	)

	if resp.Text != "" {
		slog.Debug("LLM text output", "turn", tc.turn, "text", resp.Text)
	}
	if resp.ReasoningText != "" {
		slog.Debug("LLM reasoning output", "turn", tc.turn, "reasoning", resp.ReasoningText)
	}

	for _, calledTool := range resp.ToolCalls {
		slog.Info("tool call",
			"turn", tc.turn,
			"tool", calledTool.Name,
			"call_id", calledTool.ID,
			"input", calledTool.Input,
		)
	}

	// Accumulate token usage.
	tc.result.TotalInputTokens += resp.InputTokens
	tc.result.TotalOutputTokens += resp.OutputTokens
	tc.result.Turns = tc.turn

	return tc
}

// storeMemory adds the assistant response to memory.
func (a *Agent) storeMemory(tc *TurnContext) *TurnContext {
	if tc.err != nil {
		return tc
	}

	if err := a.memory.AddAssistantResponse(tc.response); err != nil {
		tc.err = fmt.Errorf("add assistant response: %w", err)
		return tc
	}
	return tc
}

// route examines the LLM response and decides the next action.
// Sets tc.isDone=true when returning final output or an error.
func (a *Agent) route(tc *TurnContext) *TurnContext {
	decision := a.router.Route(tc.response)

	switch decision.Action {
	case ActionFinal:
		tc.result.Text = decision.Text
		tc.result.ReasoningText = tc.response.ReasoningText
		tc.isDone = true

	case ActionToolCall:
		tc.toolCalls = decision.ToolCalls

	case ActionError:
		tc.err = fmt.Errorf("routing error (turn %d): %w", tc.turn, decision.Error)
		tc.isDone = true
	}

	return tc
}

// executeTools runs all validated tool calls and appends results to memory.
// Sets tc.isLoop=true when a loop is detected.
func (a *Agent) executeTools(ctx context.Context, tc *TurnContext) *TurnContext {
	if tc.err != nil || len(tc.toolCalls) == 0 {
		return tc
	}

	results := make([]llm.ToolResult, 0, len(tc.toolCalls))
	for _, vc := range tc.toolCalls {
		if err := ctx.Err(); err != nil {
			tc.err = fmt.Errorf("context cancelled before tool %q: %w", vc.Call.Name, err)
			return tc
		}

		slog.Info("executing tool",
			"tool", vc.Call.Name,
			"call_id", vc.Call.ID,
		)

		var toolSpan *activeSpan
		if a.tracer != nil {
			toolSpan = a.tracer.StartToolSpan(vc.Call.Name, vc.Call.Input)
		}

		output, err := vc.Tool.Execute(ctx, vc.Call.Input)

		if a.tracer != nil && toolSpan != nil {
			toolSpan.EndTool(output, err)
		}

		if err != nil {
			slog.Warn("tool execution error", "tool", vc.Call.Name, "error", err)
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

	tc.toolResults = results

	// Log tool results.
	for _, tr := range results {
		isError := ""
		if tr.IsError {
			isError = " [error]"
		}
		content := tr.Content
		if len(content) > 200 {
			content = content[:200] + "... [truncated]"
		}
		slog.Debug("tool result"+isError,
			"turn", tc.turn,
			"call_id", tr.ToolCallID,
			"output", content,
		)
	}

	// Add tool results to memory.
	if err := a.memory.AddToolResult(tc.toolResults); err != nil {
		tc.err = fmt.Errorf("add tool results: %w", err)
		return tc
	}

	// Detect loops before next LLM call.
	if a.loopDetector != nil {
		loopResult := a.loopDetector.Detect()
		if loopResult.Type != LoopTypeNone {
			tc.summary.LoopsDetected++
			slog.Warn("loop detected, terminating",
				"turn", tc.turn,
				"type", loopResult.Type,
				"detail", loopResult.Detail,
			)
			tc.err = fmt.Errorf("loop detected (turn %d): %s", loopResult.Turn, loopResult.Detail)
			tc.isLoop = true
			return tc
		}
		a.loopDetector.Record(tc.turn, tc.toolCalls, tc.toolResults)
	}

	return tc
}

// ============================================================================
// streamAccumulator (package-private, used by streamingChat)

// streamAccumulator collects streaming events into a complete *llm.Response.
type streamAccumulator struct {
	text          string
	reasoningText string
	toolCalls     map[int]*accumulatedToolCall
	inputTokens   int
	outputTokens  int
	stopReason    llm.StopReason
}

type accumulatedToolCall struct {
	id, name, arguments string
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{
		toolCalls: make(map[int]*accumulatedToolCall),
	}
}

func (acc *streamAccumulator) onEvent(event *llm.PartialResponse) {
	if event == nil {
		return
	}
	switch event.Type {
	case llm.StreamEventTextDelta:
		acc.text += event.TextDelta
	case llm.StreamEventReasoningDelta:
		acc.reasoningText += event.ReasoningDelta
	case llm.StreamEventToolDelta:
		if event.ToolCallDelta == nil {
			break
		}
		tc := acc.toolCalls[event.ToolCallDelta.Index]
		if tc == nil {
			tc = &accumulatedToolCall{}
			acc.toolCalls[event.ToolCallDelta.Index] = tc
		}
		if event.ToolCallDelta.ID != "" {
			tc.id = event.ToolCallDelta.ID
		}
		if event.ToolCallDelta.Name != "" {
			tc.name = event.ToolCallDelta.Name
		}
		tc.arguments += event.ToolCallDelta.ArgumentsDelta
	case llm.StreamEventStop:
		acc.stopReason = event.StopReason
	case llm.StreamEventUsage:
		acc.inputTokens = event.InputTokens
		acc.outputTokens = event.OutputTokens
	}
}

func (acc *streamAccumulator) toResponse() *llm.Response {
	sorted := make([]schema.ToolCall, 0, len(acc.toolCalls))
	for i := 0; i < len(acc.toolCalls); i++ {
		if tc, ok := acc.toolCalls[i]; ok {
			var input map[string]interface{}
			json.Unmarshal([]byte(tc.arguments), &input)
			sorted = append(sorted, schema.ToolCall{
				ID:    tc.id,
				Name:  tc.name,
				Input: input,
			})
		}
	}
	return &llm.Response{
		StopReason:    acc.stopReason,
		Text:          acc.text,
		ReasoningText: acc.reasoningText,
		ToolCalls:     sorted,
		InputTokens:   acc.inputTokens,
		OutputTokens:  acc.outputTokens,
	}
}
