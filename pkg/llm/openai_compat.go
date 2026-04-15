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
	reasoningMode ReasoningMode
}

// OpenAICompatConfig holds the construction parameters for OpenAICompatProvider.
type OpenAICompatConfig struct {
	Name          string        // Provider name for logging/tracing (e.g. "deepseek")
	BaseURL       string        // API endpoint base URL
	APIKey        string        // API key (takes precedence over EnvKey)
	EnvKey        string        // Environment variable name to read API key from
	Model         string        // Default model name
	MaxTokens     int           // Default max completion tokens
	ReasoningMode ReasoningMode // Content classification mode for reasoning output
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
	req := openai.ChatCompletionRequest{
		Model:               p.model,
		MaxCompletionTokens: p.maxTokens,
		Messages:            convertMessagesToGoOpenAI(chatParams.Messages),
		Stream:              false,
	}
	if len(chatParams.Tools) > 0 {
		req.Tools = convertToolsToGoOpenAI(chatParams.Tools)
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
	return convertResponseFromGoOpenAI(resp, p.reasoningMode), nil
}

// ChatStream implements Provider.ChatStream using OpenAI-compatible streaming.
func (p *OpenAICompatProvider) ChatStream(ctx context.Context, chatParams ChatParams, cb func(*PartialResponse)) error {
	req := openai.ChatCompletionRequest{
		Model:               p.model,
		MaxCompletionTokens: p.maxTokens,
		Messages:            convertMessagesToGoOpenAI(chatParams.Messages),
		Stream:              true,
	}
	if len(chatParams.Tools) > 0 {
		req.Tools = convertToolsToGoOpenAI(chatParams.Tools)
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

	parser := newReasoningParser(p.reasoningMode)

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
			if choice.Delta.Content != "" {
				emitClassifiedSegments(cb, parser.Consume(choice.Delta.Content))
			}

			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					delta := &ToolCallDelta{Index: choice.Index}
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

func convertToolsToGoOpenAI(tools []schema.ToolDefinition) []openai.Tool {
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
				Parameters:  params,
			},
		}
	}
	return result
}

func convertResponseFromGoOpenAI(resp openai.ChatCompletionResponse, mode ReasoningMode) *Response {
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

	for _, tc := range choice.Message.ToolCalls {
		var input map[string]interface{}
		json.Unmarshal([]byte(tc.Function.Arguments), &input)
		result.ToolCalls = append(result.ToolCalls, schema.ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	return result
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
