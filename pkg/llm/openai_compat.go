package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sashabaranov/go-openai"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

// ============================================================================
// Generic OpenAI-compatible Provider
// ============================================================================

// OpenAICompatProvider implements Provider for any OpenAI-compatible API.
// It is embedded by concrete providers (MiniMax, Deepseek, GPT, etc.).
type OpenAICompatProvider struct {
	client        *openai.Client
	model         string
	maxTokens     int
	name          string
	reasoningMode ReasoningParseMode
}

// OpenAICompatConfig holds the construction parameters for OpenAICompatProvider.
type OpenAICompatConfig struct {
	Name          string             // Provider name for logging/tracing (e.g. "deepseek")
	BaseURL       string             // API endpoint base URL
	APIKey        string             // API key (takes precedence over EnvKey)
	EnvKey        string             // Environment variable name to read API key from
	Model         string             // Default model name
	MaxTokens     int                // Default max completion tokens
	ReasoningMode ReasoningParseMode // Content classification mode for reasoning output
}

// NewOpenAICompatProvider creates a generic OpenAI-compatible provider.
func NewOpenAICompatProvider(cfg OpenAICompatConfig) *OpenAICompatProvider {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = getEnv(cfg.EnvKey, "")
	}
	ocfg := openai.DefaultConfig(apiKey)
	if cfg.BaseURL != "" {
		ocfg.BaseURL = cfg.BaseURL
	}
	return &OpenAICompatProvider{
		client:        openai.NewClientWithConfig(ocfg),
		model:         cfg.Model,
		maxTokens:     cfg.MaxTokens,
		name:          cfg.Name,
		reasoningMode: cfg.ReasoningMode,
	}
}

func (p *OpenAICompatProvider) Name() string { return p.name }

func (p *OpenAICompatProvider) EstimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			total += len(block.Text) / 4
		}
	}
	if total == 0 {
		total = 100
	}
	return total
}

// Chat implements Provider.Chat using the OpenAI chat completions API.
func (p *OpenAICompatProvider) Chat(ctx context.Context, chatParams ChatParams) (*Response, error) {
	req, resolution, err := p.buildChatCompletionRequest(chatParams, false)
	if err != nil {
		return nil, fmt.Errorf("%s request: %w", p.name, err)
	}
	if len(chatParams.Tools) > 0 {
		req.Tools = convertToolsToGoOpenAI(chatParams.Tools, normalizeToolCallPolicy(chatParams.ToolPolicy).StrictSchema)
		applyOpenAIToolPolicy(&req, chatParams.ToolPolicy, p.name, resolution.model)
	}
	if chatParams.System != "" {
		req.Messages = append([]openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: chatParams.System},
		}, req.Messages...)
	}

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%s chat: %w", p.name, err)
	}
	converted := convertResponseFromGoOpenAI(resp, resolution.parseMode)
	converted.AppliedReasoning = resolution.applied
	return converted, nil
}

// ChatStream implements Provider.ChatStream using OpenAI-compatible streaming.
func (p *OpenAICompatProvider) ChatStream(ctx context.Context, chatParams ChatParams, cb func(*PartialResponse)) error {
	req, resolution, err := p.buildChatCompletionRequest(chatParams, true)
	if err != nil {
		return fmt.Errorf("%s request: %w", p.name, err)
	}
	if len(chatParams.Tools) > 0 {
		req.Tools = convertToolsToGoOpenAI(chatParams.Tools, normalizeToolCallPolicy(chatParams.ToolPolicy).StrictSchema)
		applyOpenAIToolPolicy(&req, chatParams.ToolPolicy, p.name, resolution.model)
	}
	if chatParams.System != "" {
		req.Messages = append([]openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: chatParams.System},
		}, req.Messages...)
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return fmt.Errorf("%s stream: %w", p.name, err)
	}
	defer stream.Close()

	parser := newReasoningParser(resolution.parseMode)
	if resolution.applied != nil {
		cb(&PartialResponse{
			Type:             StreamEventReasoningApplied,
			AppliedReasoning: resolution.applied,
		})
	}

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			emitClassifiedSegments(cb, parser.Flush())
			cb(nil) // signal stream end
			return nil
		}
		if err != nil {
			return fmt.Errorf("stream recv: %w", err)
		}

		for _, choice := range chunk.Choices {
			if resolution.parseMode == ReasoningParseModeNative && choice.Delta.ReasoningContent != "" {
				cb(&PartialResponse{
					Type:           StreamEventReasoningDelta,
					ReasoningDelta: choice.Delta.ReasoningContent,
				})
			}
			if choice.Delta.Content != "" {
				emitClassifiedSegments(cb, parser.Consume(choice.Delta.Content))
			}

			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					index := choice.Index
					if tc.Index != nil {
						index = *tc.Index
					}
					delta := &ToolCallDelta{Index: index}
					if tc.ID != "" {
						delta.ID = tc.ID
					}
					if tc.Function.Name != "" {
						delta.Name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						delta.ArgumentsDelta = tc.Function.Arguments
					}
					cb(&PartialResponse{
						Type:          StreamEventToolDelta,
						ToolCallDelta: delta,
					})
				}
			}

			if choice.FinishReason != "" {
				emitClassifiedSegments(cb, parser.Flush())
				stopReason := StopReasonEndTurn
				switch choice.FinishReason {
				case "tool_calls":
					stopReason = StopReasonToolUse
				case "length":
					stopReason = StopReasonMaxTokens
				}
				cb(&PartialResponse{
					Type:       StreamEventStop,
					StopReason: stopReason,
				})
			}
		}
	}
}

func (p *OpenAICompatProvider) buildChatCompletionRequest(chatParams ChatParams, stream bool) (openai.ChatCompletionRequest, openAIReasoningResolution, error) {
	resolution := resolveOpenAICompatReasoning(p.name, p.model, p.reasoningMode, chatParams.Reasoning)
	req := openai.ChatCompletionRequest{
		Model:               resolution.model,
		MaxCompletionTokens: p.maxTokens,
		Messages:            convertMessagesToGoOpenAI(chatParams.Messages),
		Stream:              stream,
	}
	if resolution.reasoningEffort != "" {
		req.ReasoningEffort = resolution.reasoningEffort
	}
	if err := applyOpenAIResponseFormat(&req, chatParams.ResponseFormat); err != nil {
		return openai.ChatCompletionRequest{}, resolution, err
	}
	return req, resolution, nil
}

// ============================================================================
// Conversion functions (shared by all OpenAI-compatible providers)
// ============================================================================

func convertMessagesToGoOpenAI(messages []Message) []openai.ChatCompletionMessage {
	result := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, msg := range messages {
		var role string
		switch msg.Role {
		case RoleUser:
			role = openai.ChatMessageRoleUser
		case RoleAssistant:
			role = openai.ChatMessageRoleAssistant
		case RoleTool:
			role = openai.ChatMessageRoleTool
		}

		om := openai.ChatCompletionMessage{Role: role}

		switch msg.Role {
		case RoleUser:
			var buf string
			for _, block := range msg.Content {
				if block.Type == BlockText {
					buf += block.Text
				}
			}
			if buf == "" {
				buf = " "
			}
			om.Content = buf

		case RoleTool:
			for _, block := range msg.Content {
				if block.Type == BlockToolResult {
					om.ToolCallID = block.ToolCallID
					om.Content = block.Content
				}
			}
			if om.Content == "" {
				om.Content = " "
			}

		case RoleAssistant:
			var content string
			for _, block := range msg.Content {
				if block.Type == BlockText {
					content += block.Text
				}
			}
			om.Content = content

			var toolCalls []openai.ToolCall
			for _, block := range msg.Content {
				if block.Type == BlockToolUse {
					inputJSON, _ := json.Marshal(block.Input)
					toolCalls = append(toolCalls, openai.ToolCall{
						ID:   block.ID,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      block.Name,
							Arguments: string(inputJSON),
						},
					})
				}
			}
			if len(toolCalls) > 0 {
				om.ToolCalls = toolCalls
			}
		}

		result = append(result, om)
	}
	return result
}

func convertToolsToGoOpenAI(tools []schema.ToolDefinition, strict bool) []openai.Tool {
	result := make([]openai.Tool, len(tools))
	for i, t := range tools {
		params := map[string]interface{}{
			"type":       "object",
			"properties": t.InputSchema.Properties,
		}
		if len(t.InputSchema.Required) > 0 {
			params["required"] = t.InputSchema.Required
		}
		result[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Strict:      strict,
				Parameters:  params,
			},
		}
	}
	return result
}

func convertResponseFromGoOpenAI(resp openai.ChatCompletionResponse, mode ReasoningParseMode) *Response {
	result := &Response{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		Raw:          resp,
	}

	if len(resp.Choices) == 0 {
		return result
	}

	choice := resp.Choices[0]

	switch choice.FinishReason {
	case "stop", "end_turn":
		result.StopReason = StopReasonEndTurn
	case "tool_calls":
		result.StopReason = StopReasonToolUse
	case "length":
		result.StopReason = StopReasonMaxTokens
	default:
		result.StopReason = StopReason(choice.FinishReason)
	}

	result.Text, result.ReasoningText = classifyContent(mode, choice.Message.Content)
	if mode == ReasoningParseModeNative && choice.Message.ReasoningContent != "" {
		result.ReasoningText += choice.Message.ReasoningContent
	}

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, newToolCall(tc.ID, tc.Function.Name, tc.Function.Arguments))
	}

	return result
}

func applyOpenAIResponseFormat(req *openai.ChatCompletionRequest, format *ResponseFormat) error {
	if format == nil {
		return nil
	}
	switch format.Type {
	case ResponseFormatJSONObject:
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		}
		return nil
	default:
		return fmt.Errorf("unsupported response format type %q", format.Type)
	}
}

func applyOpenAIToolPolicy(req *openai.ChatCompletionRequest, policy *ToolCallPolicy, providerName, model string) {
	if suppressOpenAIToolPolicy(providerName, model) {
		return
	}
	normalized := normalizeToolCallPolicy(policy)
	switch normalized.Choice {
	case ToolChoiceRequired:
		req.ToolChoice = "required"
	case ToolChoiceSpecific:
		req.ToolChoice = openai.ToolChoice{
			Type: openai.ToolTypeFunction,
			Function: openai.ToolFunction{
				Name: normalized.ToolName,
			},
		}
	case ToolChoiceNone:
		req.ToolChoice = "none"
	}
	if normalized.DisableParallel {
		req.ParallelToolCalls = false
	}
}

func suppressOpenAIToolPolicy(providerName, model string) bool {
	return providerName == "deepseek" && model == "deepseek-reasoner"
}

func emitClassifiedSegments(cb func(*PartialResponse), segments []contentSegment) {
	for _, segment := range segments {
		switch segment.kind {
		case contentKindReasoning:
			cb(&PartialResponse{
				Type:           StreamEventReasoningDelta,
				ReasoningDelta: segment.text,
			})
		default:
			cb(&PartialResponse{
				Type:      StreamEventTextDelta,
				TextDelta: segment.text,
			})
		}
	}
}

// ============================================================================
// Helper
// ============================================================================

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
