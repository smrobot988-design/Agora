package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"

	_ "github.com/smrobot988-design/Agora/pkg/config"
	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

const demoSystemPrompt = `You are a structured tool-call demo assistant.
Use tools as the source of truth when they are available.
If a tool is required, return the tool call only and do not answer in natural language first.
Keep tool arguments aligned with the declared schema.
After tool results, answer briefly in Chinese.`

type demoOptions struct {
	provider          string
	apiKey            string
	baseURL           string
	model             string
	task              string
	toolChoice        string
	toolName          string
	disableParallel   bool
	strictSchema      bool
	maxRepairAttempts int
	maxTurns          int
	debugLLM          bool
	showHistory       bool
	reasoningMode     string
	thinkingMode      string
	thinkingEffort    string
	thinkingBudget    int
}

func main() {
	opts := parseFlags()

	reasoningConfig, err := buildReasoningConfig(
		opts.reasoningMode,
		opts.thinkingMode,
		opts.thinkingEffort,
		opts.thinkingBudget,
	)
	if err != nil {
		log.Fatalf("reasoning config: %v", err)
	}

	toolPolicy, err := buildToolPolicy(
		opts.toolChoice,
		opts.toolName,
		opts.disableParallel,
		opts.strictSchema,
		opts.maxRepairAttempts,
	)
	if err != nil {
		log.Fatalf("tool policy: %v", err)
	}

	task := strings.TrimSpace(opts.task)
	if task == "" {
		task = defaultTask(toolPolicy)
	}

	registry := newDemoRegistry()
	mem := core.NewMemory(core.WithSystemPrompt(demoSystemPrompt))

	baseProvider := newProvider(opts.provider, opts.apiKey, opts.baseURL, opts.model)
	provider := &debugProvider{
		inner:   baseProvider,
		enabled: opts.debugLLM,
	}

	fmt.Printf("Provider: %s\n", provider.Name())
	if model := firstNonEmpty(opts.model, envModelForProvider(opts.provider)); model != "" {
		fmt.Printf("Model: %s\n", model)
	}
	if baseURL := firstNonEmpty(opts.baseURL, envBaseURLForProvider(opts.provider)); baseURL != "" {
		fmt.Printf("Base URL: %s\n", baseURL)
	}
	fmt.Printf("Task: %s\n", task)
	fmt.Printf("Tool policy: choice=%s tool=%s strict_schema=%t disable_parallel=%t max_repair_attempts=%d\n",
		displayToolChoice(toolPolicy.Choice),
		firstNonEmpty(toolPolicy.ToolName, "-"),
		toolPolicy.StrictSchema,
		toolPolicy.DisableParallel,
		toolPolicy.MaxRepairAttempts,
	)
	fmt.Printf("Available tools: %s\n\n", strings.Join(definitionNames(registry.Definitions()), ", "))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	agent := core.NewAgent(
		provider,
		mem,
		registry,
		core.WithMaxTurns(opts.maxTurns),
		core.WithReasoningConfig(reasoningConfig),
		core.WithToolCallPolicy(toolPolicy),
	)

	result, err := agent.Run(ctx, task)
	if err != nil {
		log.Fatalf("agent run: %v", err)
	}

	fmt.Println("=== Final Result ===")
	fmt.Printf("Text: %s\n", result.Text)
	if result.ReasoningText != "" {
		fmt.Printf("Reasoning: %s\n", trimText(result.ReasoningText, 240))
	}
	if result.AppliedReasoning != nil {
		fmt.Printf("Applied reasoning: provider=%s source=%s model=%s mode=%s effort=%s budget=%d parse=%s notes=%v\n",
			result.AppliedReasoning.Provider,
			result.AppliedReasoning.Source,
			result.AppliedReasoning.Model,
			result.AppliedReasoning.Mode,
			result.AppliedReasoning.Effort,
			result.AppliedReasoning.BudgetTokens,
			result.AppliedReasoning.ParseMode,
			result.AppliedReasoning.Notes,
		)
	}
	fmt.Printf("Turns: %d\n", result.Turns)
	fmt.Printf("Tokens: %d in / %d out\n", result.TotalInputTokens, result.TotalOutputTokens)
	fmt.Printf("LLM calls observed: %d\n", provider.callCount)

	toolResults, err := agent.LastToolResults()
	if err != nil {
		log.Fatalf("read last tool results: %v", err)
	}
	if len(toolResults) > 0 {
		fmt.Println("\n=== Tool Results ===")
		for i, toolResult := range toolResults {
			fmt.Printf("[%d] tool_call_id=%s is_error=%t content=%s\n",
				i+1,
				toolResult.ToolCallID,
				toolResult.IsError,
				trimText(toolResult.Content, 240),
			)
		}
	}

	if opts.showHistory {
		printHistory(mem)
	}
}

func parseFlags() demoOptions {
	opts := demoOptions{}
	flag.StringVar(&opts.provider, "provider", "claude", "Provider: claude, minimax, deepseek, doubao, kimi, glm, gpt")
	flag.StringVar(&opts.apiKey, "api-key", "", "API key, or use provider-specific env var")
	flag.StringVar(&opts.baseURL, "base-url", "", "Custom base URL, or use provider-specific env var")
	flag.StringVar(&opts.model, "model", "", "Custom model name, or use provider-specific env var")
	flag.StringVar(&opts.task, "task", "", "Task prompt; empty uses a built-in structured tool-call demo")
	flag.StringVar(&opts.toolChoice, "tool-choice", "tool", "Tool choice: auto, required, tool, none")
	flag.StringVar(&opts.toolName, "tool-name", "lookup_weather", "Specific tool name when -tool-choice=tool")
	flag.BoolVar(&opts.disableParallel, "disable-parallel", true, "Disable parallel tool calls")
	flag.BoolVar(&opts.strictSchema, "strict-schema", true, "Request provider-side strict schema when supported")
	flag.IntVar(&opts.maxRepairAttempts, "max-repair-attempts", 1, "Maximum hidden repair attempts for invalid tool calls")
	flag.IntVar(&opts.maxTurns, "max-turns", 6, "Maximum agent turns")
	flag.BoolVar(&opts.debugLLM, "debug-llm", true, "Print each LLM request/response summary")
	flag.BoolVar(&opts.showHistory, "show-history", true, "Print stored conversation history after the run")
	flag.StringVar(&opts.reasoningMode, "reasoning-mode", "auto", "Reasoning parsing mode: auto, none, think-tag, native")
	flag.StringVar(&opts.thinkingMode, "thinking-mode", "", "Provider thinking mode override: on, off, auto")
	flag.StringVar(&opts.thinkingEffort, "thinking-effort", "", "Provider thinking effort override: low, medium, high, max")
	flag.IntVar(&opts.thinkingBudget, "thinking-budget", 0, "Provider thinking budget tokens (if supported)")
	flag.Parse()
	return opts
}

func newDemoRegistry() *tool.Registry {
	registry := tool.NewRegistry()

	mustRegister(registry, &tool.Func{
		Def: schema.ToolDefinition{
			Name:        "lookup_weather",
			Description: "Return a deterministic weather payload for a location and unit.",
			InputSchema: schema.PropertySchema{
				Properties: map[string]interface{}{
					"location": map[string]interface{}{
						"type":        "string",
						"description": "City or area name in natural language",
					},
					"unit": map[string]interface{}{
						"type":        "string",
						"description": "Temperature unit",
						"enum":        []interface{}{"celsius", "fahrenheit"},
					},
					"days": map[string]interface{}{
						"type":        "integer",
						"description": "Forecast days, usually 1-3",
					},
					"include_clothing_advice": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the result should include a clothing suggestion",
					},
				},
				Required: []string{"location", "unit"},
			},
		},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			location, _ := input["location"].(string)
			unit, _ := input["unit"].(string)
			days := 1
			if rawDays, ok := input["days"].(float64); ok && rawDays > 0 {
				days = int(rawDays)
			}
			includeAdvice, _ := input["include_clothing_advice"].(bool)

			temperature := 24
			feelsLike := 22
			if unit == "fahrenheit" {
				temperature = 75
				feelsLike = 72
			}

			result := map[string]interface{}{
				"location": location,
				"unit":     unit,
				"days":     days,
				"forecast": []interface{}{
					map[string]interface{}{
						"day":         1,
						"condition":   "sunny",
						"temperature": temperature,
						"feels_like":  feelsLike,
					},
				},
			}
			if includeAdvice {
				result["clothing_advice"] = "light jacket"
			}
			return mustJSON(result), nil
		},
	})

	var ticketSeq int64
	mustRegister(registry, &tool.Func{
		Def: schema.ToolDefinition{
			Name:        "create_support_ticket",
			Description: "Create a deterministic support ticket record with structured fields.",
			InputSchema: schema.PropertySchema{
				Properties: map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Short problem summary",
					},
					"severity": map[string]interface{}{
						"type":        "string",
						"description": "Business impact severity",
						"enum":        []interface{}{"low", "medium", "high"},
					},
					"owner": map[string]interface{}{
						"type":        "string",
						"description": "Expected ticket owner",
					},
					"notify": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to notify the owner immediately",
					},
					"tags": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "Optional tags",
					},
				},
				Required: []string{"title", "severity", "owner"},
			},
		},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			id := atomic.AddInt64(&ticketSeq, 1)
			result := map[string]interface{}{
				"ticket_id": fmt.Sprintf("TICKET-%03d", id),
				"title":     input["title"],
				"severity":  input["severity"],
				"owner":     input["owner"],
				"notify":    input["notify"],
				"tags":      toStringSlice(input["tags"]),
				"status":    "created",
			}
			return mustJSON(result), nil
		},
	})

	return registry
}

func buildToolPolicy(choice, toolName string, disableParallel, strictSchema bool, maxRepairAttempts int) (llm.ToolCallPolicy, error) {
	policy := llm.ToolCallPolicy{
		DisableParallel:   disableParallel,
		StrictSchema:      strictSchema,
		MaxRepairAttempts: maxRepairAttempts,
	}

	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "", "auto":
		policy.Choice = llm.ToolChoiceAuto
	case "required":
		policy.Choice = llm.ToolChoiceRequired
	case "tool", "specific":
		policy.Choice = llm.ToolChoiceSpecific
		policy.ToolName = strings.TrimSpace(toolName)
		if policy.ToolName == "" {
			return llm.ToolCallPolicy{}, fmt.Errorf("tool-name must be set when tool-choice=tool")
		}
	case "none":
		policy.Choice = llm.ToolChoiceNone
	default:
		return llm.ToolCallPolicy{}, fmt.Errorf("unknown tool choice %q (use auto, required, tool, none)", choice)
	}

	return policy.Normalize(), nil
}

func defaultTask(policy llm.ToolCallPolicy) string {
	switch {
	case policy.Choice == llm.ToolChoiceNone:
		return "请简短解释一下 Agora 框架里 provider-agnostic tool policy 的作用。"
	case policy.ToolName == "create_support_ticket":
		return "请创建一个支持工单：标题是“Claude structured output 偶发失败”，severity=high，owner=agora-core，notify=true，并补上 tags=structured,tool-call。你必须先调用工具。"
	default:
		return "请查询上海今天的天气，使用 celsius，并给我一句简短建议。你必须先调用工具。"
	}
}

type debugProvider struct {
	inner     llm.Provider
	enabled   bool
	callCount int
}

func (p *debugProvider) Chat(ctx context.Context, params llm.ChatParams) (*llm.Response, error) {
	p.callCount++
	callID := p.callCount
	if p.enabled {
		printRequest(callID, params)
	}

	resp, err := p.inner.Chat(ctx, params)

	if p.enabled {
		printResponse(callID, resp, err)
	}
	return resp, err
}

func (p *debugProvider) ChatStream(ctx context.Context, params llm.ChatParams, cb func(*llm.PartialResponse)) error {
	return p.inner.ChatStream(ctx, params, cb)
}

func (p *debugProvider) Name() string {
	return p.inner.Name()
}

func (p *debugProvider) EstimateTokens(messages []llm.Message) int {
	return p.inner.EstimateTokens(messages)
}

func printRequest(callID int, params llm.ChatParams) {
	policy := llm.ToolCallPolicy{}.Normalize()
	if params.ToolPolicy != nil {
		policy = params.ToolPolicy.Normalize()
	}

	phase := "normal"
	if isRepairPrompt(params.Messages) {
		phase = "repair"
	}

	fmt.Printf("[llm call %d] phase=%s messages=%d tools=%d choice=%s tool=%s strict_schema=%t disable_parallel=%t max_repair_attempts=%d\n",
		callID,
		phase,
		len(params.Messages),
		len(params.Tools),
		displayToolChoice(policy.Choice),
		firstNonEmpty(policy.ToolName, "-"),
		policy.StrictSchema,
		policy.DisableParallel,
		policy.MaxRepairAttempts,
	)
}

func printResponse(callID int, resp *llm.Response, err error) {
	if err != nil {
		fmt.Printf("[llm call %d] error=%v\n", callID, err)
		return
	}
	if resp == nil {
		fmt.Printf("[llm call %d] empty response\n", callID)
		return
	}

	fmt.Printf("[llm call %d] stop=%s text_len=%d reasoning_len=%d tool_calls=%d\n",
		callID,
		resp.StopReason,
		len([]rune(resp.Text)),
		len([]rune(resp.ReasoningText)),
		len(resp.ToolCalls),
	)

	for i, call := range resp.ToolCalls {
		fmt.Printf("[llm call %d] tool[%d] name=%s raw=%s",
			callID,
			i,
			call.Name,
			trimText(call.RawArguments, 220),
		)
		if call.ParseError != "" {
			fmt.Printf(" parse_error=%s", trimText(call.ParseError, 120))
		}
		fmt.Println()
	}

	if text := strings.TrimSpace(resp.Text); text != "" {
		fmt.Printf("[llm call %d] text=%s\n", callID, trimText(text, 180))
	}
}

func printHistory(mem *core.Memory) {
	messages, err := mem.AllMessages()
	if err != nil {
		log.Fatalf("read history: %v", err)
	}
	if len(messages) == 0 {
		return
	}

	fmt.Println("\n=== Stored History ===")
	for i, message := range messages {
		for _, block := range message.Content {
			switch block.Type {
			case llm.BlockText:
				fmt.Printf("[%d] role=%s text=%s\n", i+1, message.Role, trimText(block.Text, 240))
			case llm.BlockToolUse:
				fmt.Printf("[%d] role=%s tool_use name=%s input=%s raw=%s\n",
					i+1,
					message.Role,
					block.ToolCall.Name,
					mustJSON(block.ToolCall.Input),
					trimText(block.ToolCall.RawArguments, 200),
				)
			case llm.BlockToolResult:
				fmt.Printf("[%d] role=%s tool_result tool_call_id=%s is_error=%t content=%s\n",
					i+1,
					message.Role,
					block.ToolResult.ToolCallID,
					block.ToolResult.IsError,
					trimText(block.ToolResult.Content, 240),
				)
			}
		}
	}
}

func displayToolChoice(choice llm.ToolChoiceMode) string {
	switch choice {
	case llm.ToolChoiceRequired:
		return "required"
	case llm.ToolChoiceSpecific:
		return "tool"
	case llm.ToolChoiceNone:
		return "none"
	default:
		return "auto"
	}
}

func isRepairPrompt(messages []llm.Message) bool {
	if len(messages) == 0 {
		return false
	}
	last := messages[len(messages)-1]
	if last.Role != llm.RoleUser || len(last.Content) == 0 {
		return false
	}
	return strings.HasPrefix(last.Content[0].Text, "You previously returned a tool response that Agora rejected.")
}

func mustRegister(registry *tool.Registry, t tool.Tool) {
	if err := registry.Register(t); err != nil {
		log.Fatalf("register tool %q: %v", t.Definition().Name, err)
	}
}

func mustJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func trimText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func toStringSlice(value interface{}) []string {
	switch raw := value.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), raw...)
	case []interface{}:
		result := make([]string, 0, len(raw))
		for _, item := range raw {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return []string{fmt.Sprintf("%v", raw)}
	}
}

func definitionNames(defs []schema.ToolDefinition) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

func newProvider(providerFlag, apiKey, baseURL, model string) llm.Provider {
	switch providerFlag {
	case "minimax":
		opts := []llm.MiniMaxOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("MINIMAX_API_KEY")); key != "" {
			opts = append(opts, llm.MiniMaxWithAPIKey(key))
		}
		if url := firstNonEmpty(baseURL, os.Getenv("MINIMAX_BASE_URL")); url != "" {
			opts = append(opts, llm.MiniMaxWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("MINIMAX_MODEL")); name != "" {
			opts = append(opts, llm.MiniMaxWithModel(name))
		}
		return llm.NewMiniMaxProvider(opts...)

	case "claude":
		opts := []llm.ClaudeOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("ANTHROPIC_API_KEY")); key != "" {
			opts = append(opts, llm.WithAPIKey(key))
		}
		if url := firstNonEmpty(baseURL, os.Getenv("ANTHROPIC_BASE_URL")); url != "" {
			opts = append(opts, llm.WithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("ANTHROPIC_MODEL")); name != "" {
			opts = append(opts, llm.WithModelName(name))
		}
		return llm.NewClaudeProvider(opts...)

	case "deepseek":
		opts := []llm.DeepseekOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("DEEPSEEK_API_KEY")); key != "" {
			opts = append(opts, llm.DeepseekWithAPIKey(key))
		}
		if url := firstNonEmpty(baseURL, os.Getenv("DEEPSEEK_BASE_URL")); url != "" {
			opts = append(opts, llm.DeepseekWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("DEEPSEEK_MODEL")); name != "" {
			opts = append(opts, llm.DeepseekWithModel(name))
		}
		return llm.NewDeepseekProvider(opts...)

	case "doubao":
		opts := []llm.DoubaoOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("ARK_API_KEY")); key != "" {
			opts = append(opts, llm.DoubaoWithAPIKey(key))
		}
		if url := firstNonEmpty(baseURL, os.Getenv("ARK_BASE_URL")); url != "" {
			opts = append(opts, llm.DoubaoWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("ARK_MODEL")); name != "" {
			opts = append(opts, llm.DoubaoWithModel(name))
		}
		return llm.NewDoubaoProvider(opts...)

	case "kimi":
		opts := []llm.KimiOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("MOONSHOT_API_KEY")); key != "" {
			opts = append(opts, llm.KimiWithAPIKey(key))
		}
		if url := firstNonEmpty(baseURL, os.Getenv("MOONSHOT_BASE_URL")); url != "" {
			opts = append(opts, llm.KimiWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("MOONSHOT_MODEL")); name != "" {
			opts = append(opts, llm.KimiWithModel(name))
		}
		return llm.NewKimiProvider(opts...)

	case "glm":
		opts := []llm.GLMOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("GLM_API_KEY")); key != "" {
			opts = append(opts, llm.GLMWithAPIKey(key))
		}
		if url := firstNonEmpty(baseURL, os.Getenv("GLM_BASE_URL")); url != "" {
			opts = append(opts, llm.GLMWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("GLM_MODEL")); name != "" {
			opts = append(opts, llm.GLMWithModel(name))
		}
		return llm.NewGLMProvider(opts...)

	case "gpt":
		opts := []llm.GPTOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("OPENAI_API_KEY")); key != "" {
			opts = append(opts, llm.GPTWithAPIKey(key))
		}
		if url := firstNonEmpty(baseURL, os.Getenv("OPENAI_BASE_URL")); url != "" {
			opts = append(opts, llm.GPTWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("OPENAI_MODEL")); name != "" {
			opts = append(opts, llm.GPTWithModel(name))
		}
		return llm.NewGPTProvider(opts...)

	default:
		log.Fatalf("unknown provider %q (use claude, minimax, deepseek, doubao, kimi, glm, gpt)", providerFlag)
		return nil
	}
}

func envModelForProvider(provider string) string {
	switch provider {
	case "minimax":
		return os.Getenv("MINIMAX_MODEL")
	case "claude":
		return os.Getenv("ANTHROPIC_MODEL")
	case "deepseek":
		return os.Getenv("DEEPSEEK_MODEL")
	case "doubao":
		return os.Getenv("ARK_MODEL")
	case "kimi":
		return os.Getenv("MOONSHOT_MODEL")
	case "glm":
		return os.Getenv("GLM_MODEL")
	case "gpt":
		return os.Getenv("OPENAI_MODEL")
	default:
		return ""
	}
}

func envBaseURLForProvider(provider string) string {
	switch provider {
	case "minimax":
		return os.Getenv("MINIMAX_BASE_URL")
	case "claude":
		return os.Getenv("ANTHROPIC_BASE_URL")
	case "deepseek":
		return os.Getenv("DEEPSEEK_BASE_URL")
	case "doubao":
		return os.Getenv("ARK_BASE_URL")
	case "kimi":
		return os.Getenv("MOONSHOT_BASE_URL")
	case "glm":
		return os.Getenv("GLM_BASE_URL")
	case "gpt":
		return os.Getenv("OPENAI_BASE_URL")
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func buildReasoningConfig(parseMode, thinkingMode, thinkingEffort string, thinkingBudget int) (llm.ReasoningConfig, error) {
	config := llm.ReasoningConfig{}

	switch firstNonEmpty(strings.ToLower(strings.TrimSpace(thinkingMode))) {
	case "", "inherit":
	case "on":
		config.Mode = llm.ThinkingModeOn
	case "off":
		config.Mode = llm.ThinkingModeOff
	case "auto":
		config.Mode = llm.ThinkingModeAuto
	default:
		return llm.ReasoningConfig{}, fmt.Errorf("unknown thinking mode %q (use on, off, auto)", thinkingMode)
	}

	switch firstNonEmpty(strings.ToLower(strings.TrimSpace(thinkingEffort))) {
	case "", "inherit":
	case "low":
		config.Effort = llm.ThinkingEffortLow
	case "medium":
		config.Effort = llm.ThinkingEffortMedium
	case "high":
		config.Effort = llm.ThinkingEffortHigh
	case "max":
		config.Effort = llm.ThinkingEffortMax
	default:
		return llm.ReasoningConfig{}, fmt.Errorf("unknown thinking effort %q (use low, medium, high, max)", thinkingEffort)
	}

	switch strings.ToLower(strings.TrimSpace(parseMode)) {
	case "", "auto":
	case "none":
		config.ParseMode = llm.ReasoningParseModeNone
	case "think", "think-tag", "think_tag":
		config.ParseMode = llm.ReasoningParseModeThinkTag
	case "native":
		config.ParseMode = llm.ReasoningParseModeNative
	default:
		return llm.ReasoningConfig{}, fmt.Errorf("unknown reasoning mode %q (use auto, none, think-tag, native)", parseMode)
	}

	if thinkingBudget < 0 {
		return llm.ReasoningConfig{}, fmt.Errorf("thinking-budget must be >= 0")
	}
	config.BudgetTokens = thinkingBudget
	return config.Normalize(), nil
}
