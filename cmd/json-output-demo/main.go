package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/smrobot988-design/Agora/pkg/config"
	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

const defaultJSONTask = `Return a json object only.
The json object must use this shape:
{
  "title": "string",
  "steps": ["string"],
  "risk_level": "low|medium|high",
  "needs_tool_calling": false
}
Plan how to validate DeepSeek JSON Output in Agora.`

func main() {
	providerFlag := flag.String("provider", "deepseek", "Provider: deepseek, minimax, doubao, kimi, glm, gpt")
	apiKey := flag.String("api-key", "", "API key, or use provider-specific env var")
	baseURL := flag.String("base-url", "", "Custom base URL, or use provider-specific env var")
	model := flag.String("model", "", "Custom model name, or use provider-specific env var")
	task := flag.String("task", defaultJSONTask, "Task prompt; include the word json when using json_object mode")
	responseFormatFlag := flag.String("response-format", "json_object", "Response format: json_object, none")
	reasoningMode := flag.String("reasoning-mode", "auto", "Reasoning parsing mode: auto, none, think-tag, native")
	thinkingMode := flag.String("thinking-mode", "", "Provider thinking mode override: on, off, auto")
	thinkingEffort := flag.String("thinking-effort", "", "Provider thinking effort override: low, medium, high, max")
	thinkingBudget := flag.Int("thinking-budget", 0, "Provider thinking budget tokens (if supported)")
	flag.Parse()

	reasoningConfig, err := buildReasoningConfig(*reasoningMode, *thinkingMode, *thinkingEffort, *thinkingBudget)
	if err != nil {
		log.Fatalf("reasoning config: %v", err)
	}
	responseFormat, err := buildResponseFormat(*responseFormatFlag)
	if err != nil {
		log.Fatalf("response format: %v", err)
	}

	provider := newProvider(*providerFlag, *apiKey, *baseURL, *model)
	mem := core.NewMemory(core.WithSystemPrompt("You are a JSON Output demo assistant. Return only a JSON object when JSON mode is requested."))
	registry := tool.NewRegistry()

	agentOptions := []core.AgentOption{
		core.WithMaxTurns(2),
		core.WithReasoningConfig(reasoningConfig),
	}
	if responseFormat != nil {
		agentOptions = append(agentOptions, core.WithResponseFormat(*responseFormat))
	}
	agent := core.NewAgent(provider, mem, registry, agentOptions...)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("Provider: %s\n", provider.Name())
	if resolvedModel := firstNonEmpty(*model, envModelForProvider(*providerFlag)); resolvedModel != "" {
		fmt.Printf("Model: %s\n", resolvedModel)
	}
	if resolvedBaseURL := firstNonEmpty(*baseURL, envBaseURLForProvider(*providerFlag)); resolvedBaseURL != "" {
		fmt.Printf("Base URL: %s\n", resolvedBaseURL)
	}
	if responseFormat != nil {
		fmt.Printf("Response format: %s\n", responseFormat.Type)
	} else {
		fmt.Println("Response format: none")
	}
	fmt.Printf("Task: %s\n\n", *task)

	result, err := agent.Run(ctx, *task)
	if err != nil {
		log.Fatalf("agent run: %v", err)
	}

	fmt.Println("=== Raw Text ===")
	fmt.Println(result.Text)
	if result.ReasoningText != "" {
		fmt.Println("\n=== Reasoning ===")
		fmt.Println(result.ReasoningText)
	}
	if result.AppliedReasoning != nil {
		fmt.Printf("\nApplied reasoning: provider=%s source=%s model=%s mode=%s effort=%s budget=%d parse=%s notes=%v\n",
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

	fmt.Println("\n=== JSON Parse ===")
	if pretty, err := prettyJSON(result.Text); err != nil {
		fmt.Printf("invalid JSON object: %v\n", err)
	} else {
		fmt.Println(pretty)
	}
	fmt.Printf("\nTokens: %d in / %d out\n", result.TotalInputTokens, result.TotalOutputTokens)
}

func buildResponseFormat(value string) (*llm.ResponseFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "json", "json_object":
		return &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject}, nil
	case "none":
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown response format %q (use json_object, none)", value)
	}
}

func prettyJSON(text string) (string, error) {
	var value map[string]interface{}
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(text), "", "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func newProvider(providerFlag, apiKey, baseURL, model string) llm.Provider {
	switch providerFlag {
	case "minimax":
		key := firstNonEmpty(apiKey, os.Getenv("MINIMAX_API_KEY"))
		requireAPIKey("MINIMAX_API_KEY", key)
		opts := []llm.MiniMaxOption{llm.MiniMaxWithAPIKey(key)}
		if url := firstNonEmpty(baseURL, os.Getenv("MINIMAX_BASE_URL")); url != "" {
			opts = append(opts, llm.MiniMaxWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("MINIMAX_MODEL")); name != "" {
			opts = append(opts, llm.MiniMaxWithModel(name))
		}
		return llm.NewMiniMaxProvider(opts...)

	case "deepseek":
		key := firstNonEmpty(apiKey, os.Getenv("DEEPSEEK_API_KEY"))
		requireAPIKey("DEEPSEEK_API_KEY", key)
		opts := []llm.DeepseekOption{llm.DeepseekWithAPIKey(key)}
		if url := firstNonEmpty(baseURL, os.Getenv("DEEPSEEK_BASE_URL")); url != "" {
			opts = append(opts, llm.DeepseekWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("DEEPSEEK_MODEL")); name != "" {
			opts = append(opts, llm.DeepseekWithModel(name))
		}
		return llm.NewDeepseekProvider(opts...)

	case "doubao":
		key := firstNonEmpty(apiKey, os.Getenv("ARK_API_KEY"))
		requireAPIKey("ARK_API_KEY", key)
		opts := []llm.DoubaoOption{llm.DoubaoWithAPIKey(key)}
		if url := firstNonEmpty(baseURL, os.Getenv("ARK_BASE_URL")); url != "" {
			opts = append(opts, llm.DoubaoWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("ARK_MODEL")); name != "" {
			opts = append(opts, llm.DoubaoWithModel(name))
		}
		return llm.NewDoubaoProvider(opts...)

	case "kimi":
		key := firstNonEmpty(apiKey, os.Getenv("MOONSHOT_API_KEY"))
		requireAPIKey("MOONSHOT_API_KEY", key)
		opts := []llm.KimiOption{llm.KimiWithAPIKey(key)}
		if url := firstNonEmpty(baseURL, os.Getenv("MOONSHOT_BASE_URL")); url != "" {
			opts = append(opts, llm.KimiWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("MOONSHOT_MODEL")); name != "" {
			opts = append(opts, llm.KimiWithModel(name))
		}
		return llm.NewKimiProvider(opts...)

	case "glm":
		key := firstNonEmpty(apiKey, os.Getenv("GLM_API_KEY"))
		requireAPIKey("GLM_API_KEY", key)
		opts := []llm.GLMOption{llm.GLMWithAPIKey(key)}
		if url := firstNonEmpty(baseURL, os.Getenv("GLM_BASE_URL")); url != "" {
			opts = append(opts, llm.GLMWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("GLM_MODEL")); name != "" {
			opts = append(opts, llm.GLMWithModel(name))
		}
		return llm.NewGLMProvider(opts...)

	case "gpt":
		key := firstNonEmpty(apiKey, os.Getenv("OPENAI_API_KEY"))
		requireAPIKey("OPENAI_API_KEY", key)
		opts := []llm.GPTOption{llm.GPTWithAPIKey(key)}
		if url := firstNonEmpty(baseURL, os.Getenv("OPENAI_BASE_URL")); url != "" {
			opts = append(opts, llm.GPTWithBaseURL(url))
		}
		if name := firstNonEmpty(model, os.Getenv("OPENAI_MODEL")); name != "" {
			opts = append(opts, llm.GPTWithModel(name))
		}
		return llm.NewGPTProvider(opts...)

	default:
		log.Fatalf("unknown provider %q (use deepseek, minimax, doubao, kimi, glm, gpt)", providerFlag)
		return nil
	}
}

func requireAPIKey(envKey, key string) {
	if strings.TrimSpace(key) == "" {
		log.Fatalf("%s not set (use -api-key, env, or .local.env)", envKey)
	}
}

func envModelForProvider(provider string) string {
	switch provider {
	case "minimax":
		return os.Getenv("MINIMAX_MODEL")
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
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildReasoningConfig(parseMode, thinkingMode, thinkingEffort string, thinkingBudget int) (llm.ReasoningConfig, error) {
	config := llm.ReasoningConfig{}

	switch strings.ToLower(strings.TrimSpace(thinkingMode)) {
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

	switch strings.ToLower(strings.TrimSpace(thinkingEffort)) {
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
