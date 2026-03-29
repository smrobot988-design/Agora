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
// MiniMax Provider using go-openai SDK (OpenAI-compatible API)
// ============================================================================

// MiniMaxProvider implements Provider using the MiniMax OpenAI-compatible API.
type MiniMaxProvider struct {
	client    *openai.Client
	model     string
	maxTokens int
}

// MiniMaxOption configures MiniMaxProvider.
type MiniMaxOption func(*MiniMaxProvider)

// MiniMaxWithModel sets the MiniMax model to use.
func MiniMaxWithModel(model string) MiniMaxOption {
	return func(p *MiniMaxProvider) { p.model = model }
}

// MiniMaxWithMaxTokens sets the maximum tokens for responses.
func MiniMaxWithMaxTokens(n int) MiniMaxOption {
	return func(p *MiniMaxProvider) { p.maxTokens = n }
}

// MiniMaxWithAPIKey sets the API key directly instead of reading from environment.
func MiniMaxWithAPIKey(key string) MiniMaxOption {
	return func(p *MiniMaxProvider) {
		cfg := openai.DefaultConfig(key)
		cfg.BaseURL = "https://api.minimax.chat/v1"
		p.client = openai.NewClientWithConfig(cfg)
	}
}

// MiniMaxWithBaseURL sets a custom API base URL (e.g. for proxy/relay services).
func MiniMaxWithBaseURL(url string) MiniMaxOption {
	return func(p *MiniMaxProvider) {
		cfg := openai.DefaultConfig("")
		cfg.BaseURL = url
		p.client = openai.NewClientWithConfig(cfg)
	}
}

// NewMiniMaxProvider creates a MiniMax provider.
// By default reads MINIMAX_API_KEY from environment.
func NewMiniMaxProvider(opts ...MiniMaxOption) *MiniMaxProvider {
	p := &MiniMaxProvider{
		model:     "MiniMax-M2.5",
		maxTokens: 4096,
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.client == nil {
		cfg := openai.DefaultConfig(getEnv("MINIMAX_API_KEY", ""))
		cfg.BaseURL = "https://api.minimax.chat/v1"
		p.client = openai.NewClientWithConfig(cfg)
	}
	return p
}

func (p *MiniMaxProvider) Name() string { return "minimax" }

func (p *MiniMaxProvider) EstimateTokens(messages []Message) int {
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

// Chat implements Provider.Chat using the MiniMax OpenAI-compatible API.
func (p *MiniMaxProvider) Chat(ctx context.Context, chatParams ChatParams) (*Response, error) {
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
		return nil, fmt.Errorf("minimax chat: %w", err)
	}
	return convertResponseFromGoOpenAI(resp), nil
}

// ChatStream implements Provider.ChatStream using go-openai streaming.
// It calls the callback for each streaming event.
// Returns ErrStreamingUnsupported if streaming is not available.
func (p *MiniMaxProvider) ChatStream(ctx context.Context, chatParams ChatParams, cb func(*PartialResponse)) error {
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
		return fmt.Errorf("minimax stream: %w", err)
	}
	defer stream.Close()

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			cb(nil) // signal stream end
			return nil
		}
		if err != nil {
			return fmt.Errorf("stream recv: %w", err)
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				cb(&PartialResponse{
					Type:      StreamEventTextDelta,
					TextDelta: choice.Delta.Content,
				})
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
// Custom SSE implementation (preserved, unused)
// ============================================================================

// The following code is the original hand-rolled HTTP+SSE implementation.
// It is commented out because we now use the go-openai SDK which handles
// the OpenAI-compatible API and streaming natively.
//
// To re-enable, uncomment and remove the go-openai implementation above.
// Then add these imports:
//   "bytes", "encoding/json", "net/http", "strings"
//
// ---------------------------------------------------------------------------
//
// import (
// 	"bytes"
// 	"encoding/json"
// 	"fmt"
// 	"io"
// 	"net/http"
// 	"strings"
// )
//
// type miniMaxProviderSSE struct {
// 	client    *http.Client
// 	model     string
// 	baseURL   string
// 	apiKey    string
// 	maxTokens int
// }
//
// func (p *miniMaxProviderSSE) doRequest(ctx context.Context, req minimaxRequest) ([]byte, error) { ... }
// func (p *miniMaxProviderSSE) doStreamRequest(ctx context.Context, req minimaxRequest, cb func(*PartialResponse)) error { ... }
// func parseSSEStream(body io.Reader, cb func(*PartialResponse)) error { ... }
// func processSSEData(data []byte, cb func(*PartialResponse)) { ... }
//
// ---------------------------------------------------------------------------

// ============================================================================
// Conversion functions (go-openai)
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
		result[i] = openai.Tool{
			Type:     openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema.Properties,
			},
		}
	}
	return result
}

func convertResponseFromGoOpenAI(resp openai.ChatCompletionResponse) *Response {
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

	result.Text = choice.Message.Content

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

// ============================================================================
// Helper
// ============================================================================

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
