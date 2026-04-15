// example 展示了 Agora Agent 的基础用法：无 trace、无 retry、无 loop 检测。
//
// 运行方式：
//
//	go run ./cmd/example/
//	go run ./cmd/example/ -provider minimax
//	go run ./cmd/example/ -provider claude
//
// 支持通过环境变量或 -api-key / -base-url 配置：
//
//	ANTHROPIC_API_KEY / ANTHROPIC_BASE_URL (for claude)
//	MINIMAX_API_KEY (for minimax)
//
// 或者可以在当前项目加上 .local.env 的配置文件，key=value 的形式去配置
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/smrobot988-design/Agora/pkg/config"
	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

func main() {
	providerFlag := flag.String("provider", "claude", "Provider: claude, minimax, deepseek, doubao, kimi, glm, gpt")
	apiKey := flag.String("api-key", "", "API key")
	baseURL := flag.String("base-url", "", "Base URL (claude only)")
	model := flag.String("model", "", "Model name (currently claude only; can also use ANTHROPIC_MODEL)")
	reasoningMode := flag.String("reasoning-mode", "auto", "Reasoning parsing mode: auto, none, think-tag, native")
	thinkingMode := flag.String("thinking-mode", "", "Provider thinking mode override: on, off, auto")
	thinkingEffort := flag.String("thinking-effort", "", "Provider thinking effort override: low, medium, high, max")
	thinkingBudget := flag.Int("thinking-budget", 0, "Provider thinking budget tokens (if supported)")
	flag.Parse()

	reasoningConfig, err := buildReasoningConfig(*reasoningMode, *thinkingMode, *thinkingEffort, *thinkingBudget)
	if err != nil {
		log.Fatalf("reasoning config: %v", err)
	}
	provider := newProvider(*providerFlag, *apiKey, *baseURL, *model)
	fmt.Printf("Provider: %s\n", provider.Name())

	// 注册工具
	registry := tool.NewRegistry()
	mustRegister(registry, &tool.Func{
		Def: schema.ToolDefinition{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: schema.PropertySchema{
				Properties: map[string]interface{}{
					"city": map[string]interface{}{
						"type":        "string",
						"description": "City name",
					},
				},
				Required: []string{"city"},
			},
		},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			city, _ := input["city"].(string)
			return fmt.Sprintf(`{"city": %q, "temperature": "18°C", "condition": "partly cloudy"}`, city), nil
		},
	})

	// 纯净的 Memory（无 trimmer，无特殊配置）
	mem := core.NewMemory(
		core.WithSystemPrompt("You are a helpful weather assistant. Answer concisely."),
	)

	// 纯净的 Agent：只有 provider + registry + max turns
	agent := core.NewAgent(provider, mem, registry, core.WithMaxTurns(5), core.WithReasoningConfig(reasoningConfig))

	result, err := agent.Run(context.Background(), "What's the weather in San Francisco?")
	if err != nil {
		log.Fatalf("Agent run error: %v", err)
	}

	fmt.Printf("\nResponse: %s\n", result.Text)
	if result.AppliedReasoning != nil {
		fmt.Printf("Applied reasoning: source=%s model=%s mode=%s effort=%s budget=%d parse=%s notes=%v\n",
			result.AppliedReasoning.Source,
			result.AppliedReasoning.Model,
			result.AppliedReasoning.Mode,
			result.AppliedReasoning.Effort,
			result.AppliedReasoning.BudgetTokens,
			result.AppliedReasoning.ParseMode,
			result.AppliedReasoning.Notes,
		)
	}
	fmt.Printf("Tokens: %d in / %d out\n", result.TotalInputTokens, result.TotalOutputTokens)
	fmt.Printf("Turns: %d\n", result.Turns)
}

func mustRegister(r *tool.Registry, t tool.Tool) {
	if err := r.Register(t); err != nil {
		log.Fatalf("register tool %q: %v", t.Definition().Name, err)
	}
}

// newProvider creates a Provider based on command-line flags and env vars.
func newProvider(providerFlag, apiKey, baseURL, model string) llm.Provider {
	switch providerFlag {
	case "minimax":
		opts := []llm.MiniMaxOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("MINIMAX_API_KEY")); key != "" {
			opts = append(opts, llm.MiniMaxWithAPIKey(key))
		}
		if baseURL != "" {
			opts = append(opts, llm.MiniMaxWithBaseURL(baseURL))
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
		if baseURL != "" {
			opts = append(opts, llm.DeepseekWithBaseURL(baseURL))
		}
		return llm.NewDeepseekProvider(opts...)

	case "doubao":
		opts := []llm.DoubaoOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("ARK_API_KEY")); key != "" {
			opts = append(opts, llm.DoubaoWithAPIKey(key))
		}
		if model := os.Getenv("ARK_MODEL"); model != "" {
			opts = append(opts, llm.DoubaoWithModel(model))
		}
		if baseURL != "" {
			opts = append(opts, llm.DoubaoWithBaseURL(baseURL))
		}
		return llm.NewDoubaoProvider(opts...)

	case "kimi":
		opts := []llm.KimiOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("MOONSHOT_API_KEY")); key != "" {
			opts = append(opts, llm.KimiWithAPIKey(key))
		}
		if baseURL != "" {
			opts = append(opts, llm.KimiWithBaseURL(baseURL))
		}
		return llm.NewKimiProvider(opts...)

	case "glm":
		opts := []llm.GLMOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("GLM_API_KEY")); key != "" {
			opts = append(opts, llm.GLMWithAPIKey(key))
		}
		if baseURL != "" {
			opts = append(opts, llm.GLMWithBaseURL(baseURL))
		}
		return llm.NewGLMProvider(opts...)

	case "gpt":
		opts := []llm.GPTOption{}
		if key := firstNonEmpty(apiKey, os.Getenv("OPENAI_API_KEY")); key != "" {
			opts = append(opts, llm.GPTWithAPIKey(key))
		}
		if baseURL != "" {
			opts = append(opts, llm.GPTWithBaseURL(baseURL))
		}
		return llm.NewGPTProvider(opts...)

	default:
		log.Fatalf("unknown provider %q (use claude, minimax, deepseek, doubao, kimi, glm, gpt)", providerFlag)
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
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
