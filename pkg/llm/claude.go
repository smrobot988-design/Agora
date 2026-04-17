package llm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
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

// WithModelName sets the Claude model to use from a raw model name.
func WithModelName(model string) ClaudeOption {
	return func(p *ClaudeProvider) { p.model = anthropic.Model(model) }
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

func (p *ClaudeProvider) Chat(ctx context.Context, chatParams ChatParams) (*Response, error) {
	sdkMessages, err := convertMessagesToSDK(chatParams.Messages)
	if err != nil {
		return nil, fmt.Errorf("convert messages: %w", err)
	}

	resolution := resolveClaudeReasoning(p.model, chatParams.Reasoning)
	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		Messages:  sdkMessages,
	}
	if chatParams.System != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: chatParams.System},
		}
	}
	if len(chatParams.Tools) > 0 {
		params.Tools = convertToolsToSDK(chatParams.Tools, normalizeToolCallPolicy(chatParams.ToolPolicy).StrictSchema)
		params.ToolChoice = resolveClaudeToolChoice(chatParams.ToolPolicy)
	}
	if resolution.useThinking {
		params.Thinking = resolution.thinking
	}
	if resolution.useOutput {
		params.OutputConfig = resolution.output
	}

	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("claude api: %w", err)
	}
	slog.Debug("claude api response",
		"stop_reason", msg.StopReason,
		"input_tokens", msg.Usage.InputTokens,
		"output_tokens", msg.Usage.OutputTokens)

	resp := convertResponseFromSDK(msg, resolution.parseMode)
	resp.AppliedReasoning = resolution.applied
	return resp, nil
}

func (p *ClaudeProvider) ChatStream(ctx context.Context, params ChatParams, cb func(*PartialResponse)) error {
	sdkMessages, err := convertMessagesToSDK(params.Messages)
	if err != nil {
		return fmt.Errorf("convert messages: %w", err)
	}

	resolution := resolveClaudeReasoning(p.model, params.Reasoning)
	sdkParams := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		Messages:  sdkMessages,
	}
	if params.System != "" {
		sdkParams.System = []anthropic.TextBlockParam{{Text: params.System}}
	}
	if len(params.Tools) > 0 {
		sdkParams.Tools = convertToolsToSDK(params.Tools, normalizeToolCallPolicy(params.ToolPolicy).StrictSchema)
		sdkParams.ToolChoice = resolveClaudeToolChoice(params.ToolPolicy)
	}
	if resolution.useThinking {
		sdkParams.Thinking = resolution.thinking
	}
	if resolution.useOutput {
		sdkParams.OutputConfig = resolution.output
	}

	stream := p.client.Messages.NewStreaming(ctx, sdkParams)
	defer stream.Close()

	type accumulator struct {
		index int
		id    string
		name  string
		args  string
	}
	var toolCallAcc *accumulator

	if resolution.applied != nil {
		cb(&PartialResponse{
			Type:             StreamEventReasoningApplied,
			AppliedReasoning: resolution.applied,
		})
	}

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "content_block_start":
			// Start of a content block — if it's tool_use, record its index
			if event.ContentBlock.Type == "tool_use" {
				toolUseBlock := event.ContentBlock.AsToolUse()
				toolCallAcc = &accumulator{
					index: int(event.Index),
					id:    toolUseBlock.ID,
					name:  toolUseBlock.Name,
				}
			}

		case "content_block_delta":
			delta := event.Delta
			switch delta.Type {
			case "text_delta":
				if delta.Text != "" {
					cb(&PartialResponse{
						Type:      StreamEventTextDelta,
						TextDelta: delta.Text,
					})
				}
			case "thinking_delta":
				if resolution.parseMode == ReasoningParseModeNative && delta.Thinking != "" {
					cb(&PartialResponse{
						Type:           StreamEventReasoningDelta,
						ReasoningDelta: delta.Thinking,
					})
				}
			case "input_json_delta":
				if delta.PartialJSON != "" && toolCallAcc != nil {
					toolCallAcc.args += delta.PartialJSON
					cb(&PartialResponse{
						Type: StreamEventToolDelta,
						ToolCallDelta: &ToolCallDelta{
							Index:          toolCallAcc.index,
							ArgumentsDelta: delta.PartialJSON,
						},
					})
				}
			}

		case "content_block_stop":
			// Content block finished — if it was a tool call, emit a final delta
			if toolCallAcc != nil {
				cb(&PartialResponse{
					Type: StreamEventToolDelta,
					ToolCallDelta: &ToolCallDelta{
						Index: toolCallAcc.index,
						ID:    toolCallAcc.id,
						Name:  toolCallAcc.name,
					},
				})
				toolCallAcc = nil
			}

		case "message_delta":
			if event.Usage.OutputTokens > 0 {
				cb(&PartialResponse{
					Type:         StreamEventUsage,
					InputTokens:  int(event.Usage.InputTokens),
					OutputTokens: int(event.Usage.OutputTokens),
				})
			}
			stopReason := StopReasonEndTurn
			switch event.Delta.StopReason {
			case "end_turn":
				stopReason = StopReasonEndTurn
			case "tool_use":
				stopReason = StopReasonToolUse
			case "max_tokens":
				stopReason = StopReasonMaxTokens
			}
			cb(&PartialResponse{
				Type:       StreamEventStop,
				StopReason: stopReason,
			})
		}
	}

	if err := stream.Err(); err != nil {
		return fmt.Errorf("claude stream: %w", err)
	}

	cb(nil) // signal stream end
	return nil
}

// toolCallAccumulator holds in-progress tool call data during streaming.
type toolCallAccumulator = struct {
	index int
	id    string
	name  string
	args  string
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
func convertToolsToSDK(tools []schema.ToolDefinition, strict bool) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, len(tools))
	for i, t := range tools {
		result[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				Strict:      param.NewOpt(strict),
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
func convertResponseFromSDK(msg *anthropic.Message, parseMode ReasoningParseMode) *Response {
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
		case "thinking":
			if parseMode == ReasoningParseModeNative {
				resp.ReasoningText += block.Thinking
			}
		case BlockToolUse:
			resp.ToolCalls = append(resp.ToolCalls, newToolCall(block.ID, block.Name, string(block.Input)))
		}
	}

	return resp
}

func resolveClaudeToolChoice(policy *ToolCallPolicy) anthropic.ToolChoiceUnionParam {
	normalized := normalizeToolCallPolicy(policy)
	switch normalized.Choice {
	case ToolChoiceRequired:
		return anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{
				DisableParallelToolUse: param.NewOpt(normalized.DisableParallel),
			},
		}
	case ToolChoiceSpecific:
		choice := anthropic.ToolChoiceParamOfTool(normalized.ToolName)
		if choice.OfTool != nil {
			choice.OfTool.DisableParallelToolUse = param.NewOpt(normalized.DisableParallel)
		}
		return choice
	case ToolChoiceNone:
		none := anthropic.NewToolChoiceNoneParam()
		return anthropic.ToolChoiceUnionParam{OfNone: &none}
	default:
		if normalized.DisableParallel {
			return anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{
					DisableParallelToolUse: param.NewOpt(true),
				},
			}
		}
		return anthropic.ToolChoiceUnionParam{}
	}
}
