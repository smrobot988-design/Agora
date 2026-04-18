package llm

import (
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

// --- Interface compliance ---

var _ Provider = (*OpenAICompatProvider)(nil)

// --- Unit tests ---

func TestOpenAICompatProviderName(t *testing.T) {
	p := NewOpenAICompatProvider(OpenAICompatConfig{Name: "test-provider"})
	if p.Name() != "test-provider" {
		t.Fatalf("expected 'test-provider', got %s", p.Name())
	}
}

func TestOpenAICompatProviderEstimateTokens(t *testing.T) {
	p := NewOpenAICompatProvider(OpenAICompatConfig{Name: "test"})
	msgs := []Message{NewTextMessage(RoleUser, "Hello world, this is a test.")}
	tokens := p.EstimateTokens(msgs)
	if tokens <= 0 {
		t.Fatalf("expected positive token estimate, got %d", tokens)
	}
}

func TestOpenAICompatProviderEstimateTokensEmpty(t *testing.T) {
	p := NewOpenAICompatProvider(OpenAICompatConfig{Name: "test"})
	tokens := p.EstimateTokens([]Message{})
	if tokens != 100 {
		t.Fatalf("expected default 100, got %d", tokens)
	}
}

// --- Conversion function tests (moved from minimax_test.go) ---

func TestConvertMessagesToOpenAI(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "Hello"),
		NewTextMessage(RoleAssistant, "Hi there"),
	}
	result := convertMessagesToGoOpenAI(messages)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %s, %s", result[0].Role, result[1].Role)
	}
	if result[0].Content != "Hello" || result[1].Content != "Hi there" {
		t.Fatalf("unexpected content: %s, %s", result[0].Content, result[1].Content)
	}
}

func TestConvertMessagesToOpenAIWithToolResult(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "What's the weather?"),
		{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{Type: BlockToolUse, ToolCall: schema.ToolCall{ID: "call_123", Name: "get_weather", Input: map[string]interface{}{"location": "Tokyo"}}},
			},
		},
		{
			Role: RoleTool,
			Content: []ContentBlock{
				{Type: BlockToolResult, ToolResult: ToolResult{ToolCallID: "call_123", Content: `{"temp": "20°C"}`}},
			},
		},
	}
	result := convertMessagesToGoOpenAI(messages)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if result[2].Role != "tool" {
		t.Fatalf("expected role 'tool', got %s", result[2].Role)
	}
	if result[2].ToolCallID != "call_123" {
		t.Fatalf("expected tool_call_id 'call_123', got %s", result[2].ToolCallID)
	}
	if result[2].Content != `{"temp": "20°C"}` {
		t.Fatalf("expected tool result content, got %s", result[2].Content)
	}
}

func TestConvertToolsToOpenAI(t *testing.T) {
	tools := []schema.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather",
			InputSchema: schema.PropertySchema{
				Properties: map[string]interface{}{
					"location": map[string]interface{}{
						"type":        "string",
						"description": "City name",
					},
				},
				Required: []string{"location"},
			},
		},
	}
	result := convertToolsToGoOpenAI(tools, true)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "function" {
		t.Fatalf("expected type 'function', got %s", result[0].Type)
	}
	if result[0].Function.Name != "get_weather" {
		t.Fatalf("expected name 'get_weather', got %s", result[0].Function.Name)
	}
	if !result[0].Function.Strict {
		t.Fatal("expected strict schema to be enabled")
	}
}

func TestConvertResponseFromGoOpenAIClassifiesReasoning(t *testing.T) {
	resp := openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "stop",
				Message: openai.ChatCompletionMessage{
					Content: "<think>hidden</think>visible",
				},
			},
		},
	}

	result := convertResponseFromGoOpenAI(resp, ReasoningModeThinkTag)
	if result.Text != "visible" {
		t.Fatalf("expected visible text, got %q", result.Text)
	}
	if result.ReasoningText != "hidden" {
		t.Fatalf("expected reasoning text, got %q", result.ReasoningText)
	}
}

func TestConvertResponseFromGoOpenAINativeReasoning(t *testing.T) {
	resp := openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "stop",
				Message: openai.ChatCompletionMessage{
					Content:          "visible",
					ReasoningContent: "hidden",
				},
			},
		},
	}

	result := convertResponseFromGoOpenAI(resp, ReasoningModeNative)
	if result.Text != "visible" {
		t.Fatalf("expected visible text, got %q", result.Text)
	}
	if result.ReasoningText != "hidden" {
		t.Fatalf("expected native reasoning text, got %q", result.ReasoningText)
	}
}

func TestConvertResponseFromGoOpenAIPreservesRawToolArguments(t *testing.T) {
	resp := openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "tool_calls",
				Message: openai.ChatCompletionMessage{
					ToolCalls: []openai.ToolCall{
						{
							ID:   "call_1",
							Type: openai.ToolTypeFunction,
							Function: openai.FunctionCall{
								Name:      "get_weather",
								Arguments: "```json\n{\"location\":\"Tokyo\",}\n```",
							},
						},
					},
				},
			},
		},
	}

	result := convertResponseFromGoOpenAI(resp, ReasoningModeNone)
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].RawArguments == "" {
		t.Fatal("expected raw arguments to be preserved")
	}
	if result.ToolCalls[0].Input["location"] != "Tokyo" {
		t.Fatalf("expected repaired JSON arguments, got %#v", result.ToolCalls[0].Input)
	}
	if result.ToolCalls[0].ParseError != "" {
		t.Fatalf("expected parse repair to succeed, got %q", result.ToolCalls[0].ParseError)
	}
}

func TestApplyOpenAIToolPolicy(t *testing.T) {
	req := openai.ChatCompletionRequest{}
	applyOpenAIToolPolicy(&req, &ToolCallPolicy{
		Choice:          ToolChoiceSpecific,
		ToolName:        "get_weather",
		DisableParallel: true,
	}, "deepseek", "deepseek-chat")

	choice, ok := req.ToolChoice.(openai.ToolChoice)
	if !ok {
		t.Fatalf("expected ToolChoice struct, got %#v", req.ToolChoice)
	}
	if choice.Function.Name != "get_weather" {
		t.Fatalf("expected get_weather tool choice, got %q", choice.Function.Name)
	}
	parallel, ok := req.ParallelToolCalls.(bool)
	if !ok || parallel {
		t.Fatalf("expected parallel tool calls to be disabled, got %#v", req.ParallelToolCalls)
	}
}

func TestApplyOpenAIToolPolicySuppressesDeepSeekReasonerToolChoice(t *testing.T) {
	req := openai.ChatCompletionRequest{}
	applyOpenAIToolPolicy(&req, &ToolCallPolicy{
		Choice:          ToolChoiceSpecific,
		ToolName:        "get_weather",
		DisableParallel: true,
	}, "deepseek", "deepseek-reasoner")

	if req.ToolChoice != nil {
		t.Fatalf("expected tool_choice to be suppressed, got %#v", req.ToolChoice)
	}
	if req.ParallelToolCalls != nil {
		t.Fatalf("expected parallel_tool_calls to be suppressed, got %#v", req.ParallelToolCalls)
	}
}

func TestBuildChatCompletionRequestDeepSeekThinkingSuppressesToolChoice(t *testing.T) {
	provider := NewOpenAICompatProvider(OpenAICompatConfig{
		Name:      "deepseek",
		Model:     "deepseek-chat",
		MaxTokens: 4096,
	})

	req, resolution, err := provider.buildChatCompletionRequest(ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "hi")},
		Reasoning: &ReasoningConfig{
			Mode: ThinkingModeOn,
		},
	}, false)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resolution.model != "deepseek-reasoner" {
		t.Fatalf("expected deepseek-reasoner model, got %q", resolution.model)
	}

	applyOpenAIToolPolicy(&req, &ToolCallPolicy{
		Choice:          ToolChoiceSpecific,
		ToolName:        "get_weather",
		DisableParallel: true,
	}, provider.name, resolution.model)

	if req.ToolChoice != nil {
		t.Fatalf("expected reasoning path tool_choice to be suppressed, got %#v", req.ToolChoice)
	}
}

func TestBuildChatCompletionRequestAppliesJSONResponseFormat(t *testing.T) {
	provider := NewOpenAICompatProvider(OpenAICompatConfig{
		Name:      "deepseek",
		Model:     "deepseek-reasoner",
		MaxTokens: 4096,
	})

	req, _, err := provider.buildChatCompletionRequest(ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "return json")},
		ResponseFormat: &ResponseFormat{
			Type: ResponseFormatJSONObject,
		},
	}, false)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if req.ResponseFormat == nil {
		t.Fatal("expected response_format to be set")
	}
	if req.ResponseFormat.Type != openai.ChatCompletionResponseFormatTypeJSONObject {
		t.Fatalf("expected json_object response format, got %q", req.ResponseFormat.Type)
	}
}

func TestBuildChatCompletionRequestRejectsUnsupportedResponseFormat(t *testing.T) {
	provider := NewOpenAICompatProvider(OpenAICompatConfig{
		Name:      "deepseek",
		Model:     "deepseek-chat",
		MaxTokens: 4096,
	})

	_, _, err := provider.buildChatCompletionRequest(ChatParams{
		Messages: []Message{NewTextMessage(RoleUser, "return yaml")},
		ResponseFormat: &ResponseFormat{
			Type: ResponseFormatType("yaml"),
		},
	}, false)
	if err == nil {
		t.Fatal("expected unsupported response format error")
	}
}
