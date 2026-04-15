// with-trace 展示了 Agora Agent 完整的特性，支持重试、循环检测、Trace、总结：
// RetryProvider、LoopDetector、Tracer、Summarizer。
//
// 运行方式：
//
//	go run ./cmd/with-trace/
//	go run ./cmd/with-trace/ -provider minimax
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/smrobot988-design/Agora/pkg/config"
	"github.com/smrobot988-design/Agora/pkg/core"
	agoratrace "github.com/smrobot988-design/Agora/pkg/core/trace"
	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/memory/store"
	"github.com/smrobot988-design/Agora/pkg/memory/trimmer"
	"github.com/smrobot988-design/Agora/pkg/tool"
	"github.com/smrobot988-design/Agora/pkg/tool/builtin"
)

func main() {
	provider, reasoningConfig := newProvider()
	fmt.Printf("Provider: %s\n", provider.Name())

	// RetryProvider：指数退避重试
	retryPolicy := core.DefaultRetryPolicy()
	retryPolicy.MaxRetries = 2
	retryPolicy.InitialDelay = 500 * 1e6 // 500ms

	// Memory：持久化 store + token budget trimmer
	mem := core.NewMemory(
		core.WithStore(store.NewInMemory()),
		core.WithTrimmer(&trimmer.NoOp{}),
		core.WithSystemPrompt("You are a helpful assistant that can use tools, but you answer need use chinese.即必须使用中文回答！！"),
	)

	// Registry：内置工具
	registry := tool.NewRegistry()
	mustRegister(registry, builtin.NewReadFile())
	mustRegister(registry, builtin.NewRunCommand(
		builtin.WithDefaultTimeout(5),
		builtin.WithMaxOutput(50*1024),
	))

	// Agent：启用全部失败重试、循环检测、Trace 总结等特性
	agent := core.NewAgent(
		core.NewRetryProvider(provider, retryPolicy),
		mem,
		registry,
		core.WithMaxTurns(10),
		core.WithLoopDetector(),
		core.WithReasoningConfig(reasoningConfig),
		core.WithTracer(&jsonExporter{}),
		core.WithSummarizer(),
	)

	tasks := []string{
		"Hello, who are you?",
		"Read the file go.mod in this project",
		"List files in this project using run_command",
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	for i, task := range tasks {
		idx := i + 1
		fmt.Printf("[task %d] starting: %s\n", idx, task)
		result, err := agent.Run(ctx, task)
		if err != nil {
			fmt.Printf("[task %d] FAILED: %v\n", idx, err)
			continue
		}
		fmt.Printf("[task %d] done: %q (%d turns)\n", idx, result.Text, result.Turns)
	}

	fmt.Println("\nCheck for trace-*.json files in current directory.")
}

// jsonExporter writes each trace to a separate JSON file.
type jsonExporter struct{}

func (e *jsonExporter) Export(t *agoratrace.Trace) error {
	data, err := t.MarshalJSON()
	if err != nil {
		return err
	}
	return os.WriteFile("trace-"+t.TraceID+".json", data, 0644)
}

func mustRegister(r *tool.Registry, t tool.Tool) {
	if err := r.Register(t); err != nil {
		log.Fatalf("register tool %q: %v", t.Definition().Name, err)
	}
}

func newProvider() (llm.Provider, llm.ReasoningConfig) {
	providerFlag := flag.String("provider", "claude", "Provider: claude, minimax, deepseek, doubao, kimi, glm, gpt")
	apiKey := flag.String("api-key", "", "API key")
	baseURL := flag.String("base-url", "", "Base URL")
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

	switch *providerFlag {
	case "minimax":
		opts := []llm.MiniMaxOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("MINIMAX_API_KEY")); key != "" {
			opts = append(opts, llm.MiniMaxWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.MiniMaxWithBaseURL(*baseURL))
		}
		return llm.NewMiniMaxProvider(opts...), reasoningConfig

	case "claude":
		opts := []llm.ClaudeOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("ANTHROPIC_API_KEY")); key != "" {
			opts = append(opts, llm.WithAPIKey(key))
		}
		if url := firstNonEmpty(*baseURL, os.Getenv("ANTHROPIC_BASE_URL")); url != "" {
			opts = append(opts, llm.WithBaseURL(url))
		}
		if name := firstNonEmpty(*model, os.Getenv("ANTHROPIC_MODEL")); name != "" {
			opts = append(opts, llm.WithModelName(name))
		}
		return llm.NewClaudeProvider(opts...), reasoningConfig

	case "deepseek":
		opts := []llm.DeepseekOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("DEEPSEEK_API_KEY")); key != "" {
			opts = append(opts, llm.DeepseekWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.DeepseekWithBaseURL(*baseURL))
		}
		return llm.NewDeepseekProvider(opts...), reasoningConfig

	case "doubao":
		opts := []llm.DoubaoOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("ARK_API_KEY")); key != "" {
			opts = append(opts, llm.DoubaoWithAPIKey(key))
		}
		if model := os.Getenv("ARK_MODEL"); model != "" {
			opts = append(opts, llm.DoubaoWithModel(model))
		}
		if *baseURL != "" {
			opts = append(opts, llm.DoubaoWithBaseURL(*baseURL))
		}
		return llm.NewDoubaoProvider(opts...), reasoningConfig

	case "kimi":
		opts := []llm.KimiOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("MOONSHOT_API_KEY")); key != "" {
			opts = append(opts, llm.KimiWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.KimiWithBaseURL(*baseURL))
		}
		return llm.NewKimiProvider(opts...), reasoningConfig

	case "glm":
		opts := []llm.GLMOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("GLM_API_KEY")); key != "" {
			opts = append(opts, llm.GLMWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.GLMWithBaseURL(*baseURL))
		}
		return llm.NewGLMProvider(opts...), reasoningConfig

	case "gpt":
		opts := []llm.GPTOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("OPENAI_API_KEY")); key != "" {
			opts = append(opts, llm.GPTWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.GPTWithBaseURL(*baseURL))
		}
		return llm.NewGPTProvider(opts...), reasoningConfig

	default:
		log.Fatalf("unknown provider %q (use claude, minimax, deepseek, doubao, kimi, glm, gpt)", *providerFlag)
		return nil, llm.ReasoningConfig{}
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
