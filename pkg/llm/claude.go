package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

// ClaudeProvider implements Provider using the Anthropic Claude API.
type ClaudeProvider struct {
	client    anthropic.Client
	model     anthropic.Model
	maxTokens int64
	baseURL   string
	apiKey    string
}

// ClaudeOption configures ClaudeProvider.
type ClaudeOption func(*ClaudeProvider)

// WithModel sets the Claude model to use.
func WithModel(model anthropic.Model) ClaudeOption {
	return func(p *ClaudeProvider) { p.model = model }
}

// WithMaxTokens sets the maximum tokens for responses.
func WithMaxTokens(n int64) ClaudeOption {
	return func(p *ClaudeProvider) { p.maxTokens = n }
}

// WithAPIKey sets the API key directly instead of reading from environment.
func WithAPIKey(key string) ClaudeOption {
	return func(p *ClaudeProvider) { p.apiKey = key }
}

// WithBaseURL sets a custom API base URL (e.g. for proxy/relay services).
func WithBaseURL(url string) ClaudeOption {
	return func(p *ClaudeProvider) { p.baseURL = url }
}

// NewClaudeProvider creates a Claude provider.
// By default reads ANTHROPIC_API_KEY from environment.
func NewClaudeProvider(opts ...ClaudeOption) *ClaudeProvider {
	p := &ClaudeProvider{
		model:     anthropic.ModelClaudeSonnet4_6,
		maxTokens: 4096,
	}
	for _, opt := range opts {
		opt(p)
	}
	var clientOpts []option.RequestOption
	if p.apiKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(p.apiKey))
	}
	if p.baseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(p.baseURL))
	}
	p.client = anthropic.NewClient(clientOpts...)
	return p
}

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) EstimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			total += len(block.Text)/4 + len(block.Content)/4 // TODO：目前的计算方式有问题，后续要考虑更加精细话的计算，可以调用 Anthropic 的 token counting API
		}
	}
	if total == 0 {
		total = 100
	}
	return total
}

func (p *ClaudeProvider) Chat(ctx context.Context, messages []Message, tools []schema.ToolDefinition) (*Response, error) {
	sdkMessages, err := convertMessagesToSDK(messages)
	if err != nil {
		return nil, fmt.Errorf("convert messages: %w", err)
	}

	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		Messages:  sdkMessages,
	}
	if len(tools) > 0 {
		params.Tools = convertToolsToSDK(tools)
	}

	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("claude api: %w", err)
	}

	return convertResponseFromSDK(msg), nil
}

// convertMessagesToSDK converts our Message slice to SDK MessageParam slice.
func convertMessagesToSDK(messages []Message) ([]anthropic.MessageParam, error) {
	result := make([]anthropic.MessageParam, 0, len(messages))
	for _, msg := range messages {
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
		for _, block := range msg.Content {
			switch block.Type {
			case BlockText:
				blocks = append(blocks, anthropic.NewTextBlock(block.Text))
			case BlockToolUse:
				blocks = append(blocks, anthropic.NewToolUseBlock(block.ID, block.Input, block.Name))
			case BlockToolResult:
				// ToolCallID is our framework term; Claude API calls it tool_use_id
				blocks = append(blocks, anthropic.NewToolResultBlock(block.ToolCallID, block.Content, block.IsError))
			default:
				return nil, fmt.Errorf("unknown content block type: %s", block.Type)
			}
		}
		switch msg.Role {
		case RoleUser:
			result = append(result, anthropic.NewUserMessage(blocks...))
		case RoleAssistant:
			result = append(result, anthropic.NewAssistantMessage(blocks...))
		case RoleTool:
			// Claude API: tool results are sent as user messages
			result = append(result, anthropic.NewUserMessage(blocks...))
		default:
			return nil, fmt.Errorf("unknown message role: %s", msg.Role)
		}
	}
	return result, nil
}

// convertToolsToSDK converts our ToolDefinition slice to SDK ToolUnionParam slice.
func convertToolsToSDK(tools []schema.ToolDefinition) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, len(tools))
	for i, t := range tools {
		result[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: t.InputSchema.Properties,
					Required:   t.InputSchema.Required,
				},
			},
		}
	}
	return result
}

// convertResponseFromSDK extracts our Response from an SDK Message.
func convertResponseFromSDK(msg *anthropic.Message) *Response {
	resp := &Response{
		StopReason:   StopReason(msg.StopReason),
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
		Raw:          msg,
	}

	for _, block := range msg.Content {
		switch BlockType(block.Type) {
		case BlockText:
			if resp.Text != "" {
				resp.Text += "\n"
			}
			resp.Text += block.Text
		case BlockToolUse:
			inputMap := make(map[string]interface{})
			if len(block.Input) > 0 {
				json.Unmarshal(block.Input, &inputMap)
			}
			resp.ToolCalls = append(resp.ToolCalls, schema.ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: inputMap,
			})
		}
	}

	return resp
}
