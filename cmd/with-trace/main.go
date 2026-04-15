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
	provider := newProvider()
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

func newProvider() llm.Provider {
	providerFlag := flag.String("provider", "claude", "Provider: claude, minimax, deepseek, doubao, kimi, glm, gpt")
	apiKey := flag.String("api-key", "", "API key")
	baseURL := flag.String("base-url", "", "Base URL")
	model := flag.String("model", "", "Model name (currently claude only; can also use ANTHROPIC_MODEL)")
	flag.Parse()

	switch *providerFlag {
	case "minimax":
		opts := []llm.MiniMaxOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("MINIMAX_API_KEY")); key != "" {
			opts = append(opts, llm.MiniMaxWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.MiniMaxWithBaseURL(*baseURL))
		}
		return llm.NewMiniMaxProvider(opts...)

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
		return llm.NewClaudeProvider(opts...)

	case "deepseek":
		opts := []llm.DeepseekOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("DEEPSEEK_API_KEY")); key != "" {
			opts = append(opts, llm.DeepseekWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.DeepseekWithBaseURL(*baseURL))
		}
		return llm.NewDeepseekProvider(opts...)

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
		return llm.NewDoubaoProvider(opts...)

	case "kimi":
		opts := []llm.KimiOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("MOONSHOT_API_KEY")); key != "" {
			opts = append(opts, llm.KimiWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.KimiWithBaseURL(*baseURL))
		}
		return llm.NewKimiProvider(opts...)

	case "glm":
		opts := []llm.GLMOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("GLM_API_KEY")); key != "" {
			opts = append(opts, llm.GLMWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.GLMWithBaseURL(*baseURL))
		}
		return llm.NewGLMProvider(opts...)

	case "gpt":
		opts := []llm.GPTOption{}
		if key := firstNonEmpty(*apiKey, os.Getenv("OPENAI_API_KEY")); key != "" {
			opts = append(opts, llm.GPTWithAPIKey(key))
		}
		if *baseURL != "" {
			opts = append(opts, llm.GPTWithBaseURL(*baseURL))
		}
		return llm.NewGPTProvider(opts...)

	default:
		log.Fatalf("unknown provider %q (use claude, minimax, deepseek, doubao, kimi, glm, gpt)", *providerFlag)
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
