package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

func (a *Agent) normalizeToolResponse(ctx context.Context, tc *TurnContext) *TurnContext {
	if tc.err != nil || tc.response == nil {
		return tc
	}

	policy := llm.ToolCallPolicy{}
	if effective := a.toolCallPolicyForMessages(tc.messages); effective != nil {
		policy = effective.Normalize()
	} else {
		policy = policy.Normalize()
	}

	for attempts := 0; ; attempts++ {
		decision := a.router.Route(tc.response, a.toolCallPolicyForMessages(tc.messages))
		if decision.Action != ActionError {
			if decision.Action == ActionToolCall {
				tc.response.ToolCalls = make([]schema.ToolCall, len(decision.ToolCalls))
				for i, validated := range decision.ToolCalls {
					tc.response.ToolCalls[i] = validated.Call
				}
			}
			tc.decision = &decision
			return tc
		}

		var toolErr *ToolCallError
		if !errors.As(decision.Error, &toolErr) || attempts >= policy.MaxRepairAttempts {
			tc.err = fmt.Errorf("routing error (turn %d): %w", tc.turn, decision.Error)
			return tc
		}

		repairedResp, err := a.repairToolResponse(ctx, tc.messages, tc.response, toolErr)
		if err != nil {
			tc.err = fmt.Errorf("repair tool call (turn %d): %w", tc.turn, err)
			return tc
		}
		tc.response = repairedResp
	}
}

func (a *Agent) repairToolResponse(ctx context.Context, history []llm.Message, failedResp *llm.Response, toolErr *ToolCallError) (*llm.Response, error) {
	repairPolicy := a.effectiveRepairPolicy(toolErr)
	repairPrompt, err := buildToolRepairPrompt(toolErr, a.registry.Definitions(), failedResp)
	if err != nil {
		return nil, err
	}

	messages := append([]llm.Message(nil), history...)
	messages = append(messages, llm.NewTextMessage(llm.RoleUser, repairPrompt))

	return a.provider.Chat(ctx, llm.ChatParams{
		System:         a.memory.SystemPrompt(),
		Messages:       messages,
		Tools:          a.registry.Definitions(),
		ToolPolicy:     &repairPolicy,
		Reasoning:      a.reasoningConfig,
		ResponseFormat: a.responseFormat,
	})
}

func (a *Agent) effectiveRepairPolicy(toolErr *ToolCallError) llm.ToolCallPolicy {
	policy := llm.ToolCallPolicy{}
	if a.toolCallPolicy != nil {
		policy = a.toolCallPolicy.Normalize()
	} else {
		policy = policy.Normalize()
	}

	policy.MaxRepairAttempts = 0
	policy.DisableParallel = true

	switch {
	case toolErr != nil && toolErr.ToolName != "":
		policy.Choice = llm.ToolChoiceSpecific
		policy.ToolName = toolErr.ToolName
	case policy.Choice == llm.ToolChoiceAuto:
		policy.Choice = llm.ToolChoiceRequired
	}

	return policy.Normalize()
}

func (a *Agent) toolCallPolicyForMessages(messages []llm.Message) *llm.ToolCallPolicy {
	if a.toolCallPolicy == nil {
		return nil
	}
	policy := a.toolCallPolicy.Normalize()
	if len(messages) > 0 && messages[len(messages)-1].Role == llm.RoleTool {
		policy.Choice = llm.ToolChoiceAuto
	}
	return &policy
}

func buildToolRepairPrompt(toolErr *ToolCallError, defs []schema.ToolDefinition, failedResp *llm.Response) (string, error) {
	report := map[string]interface{}{
		"error_code":      toolErr.Code,
		"message":         toolErr.Error(),
		"required_tool":   toolErr.ToolName,
		"available_tools": toolNames(defs),
	}
	if toolErr.Call != nil {
		report["rejected_call"] = toolErr.Call
	}
	if len(toolErr.Issues) > 0 {
		report["validation_issues"] = toolErr.Issues
	}
	if failedResp != nil && failedResp.Text != "" {
		report["previous_text"] = failedResp.Text
	}

	payload, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("marshal repair report: %w", err)
	}

	if toolErr.Code == ToolCallErrorToolCallsForbidden {
		return strings.TrimSpace(
			"You previously returned a tool call that Agora rejected.\n" +
				"You must respond with the final answer only.\n" +
				"Do not call any tool.\n" +
				"Do not explain the error.\n" +
				"Failure report:\n" + string(payload),
		), nil
	}

	return strings.TrimSpace(
		"You previously returned a tool response that Agora rejected.\n" +
			"You must respond with a corrected tool call only.\n" +
			"Do not answer in natural language.\n" +
			"Do not explain the error.\n" +
			"Keep arguments strictly aligned with the declared tool schema.\n" +
			"Failure report:\n" + string(payload),
	), nil
}

func toolNames(defs []schema.ToolDefinition) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}
