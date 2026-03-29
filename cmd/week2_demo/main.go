// Week 2 特性演示：RetryPolicy + LoopDetector + Trace + Summary
//
// 运行方式：
//
//	export ANTHROPIC_API_KEY=your_key_here
//	go run ./cmd/week2_demo/
//
// 演示内容：
//  1. RetryProvider — LLM 调用失败时的重试行为
//  2. LoopDetector — 连续相同工具调用触发循环检测
//  3. Tracer — 每次 Run 结束后写入 trace-<id>.json
//  4. Summarizer — 每次 Run 结束后打印 "Run complete" 摘要
package main

import (
	"context"
	"log/slog"
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
	// 配置 slog 为 JSON 格式，方便查看结构化字段
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		slog.Warn("ANTHROPIC_API_KEY not set, demo will use mock provider")
	}
	// 创建 Provider
	var provider llm.Provider
	if apiKey != "" {
		provider = llm.NewClaudeProvider(
			llm.WithAPIKey(apiKey),
			llm.WithModel("claude-sonnet-4-6"),
		)
	} else {
		slog.Warn("using no-op mock provider — set ANTHROPIC_API_KEY for real LLM calls")
		provider = &mockProvider{resp: &llm.Response{Text: "mock response"}}
	}

	// RetryProvider 包装：指数退避重试
	retryPolicy := core.DefaultRetryPolicy()
	retryPolicy.MaxRetries = 2
	retryPolicy.InitialDelay = 100 * 1e6 // 100ms（演示用）

	// 创建 Memory
	mem := core.NewMemory(
		core.WithStore(store.NewInMemory()),
		core.WithTrimmer(&trimmer.NoOp{}),
		core.WithSystemPrompt("You are a helpful assistant that can use tools."),
	)

	// 创建 Registry 并注册内置工具
	registry := tool.NewRegistry()
	registry.Register(builtin.NewReadFile())
	registry.Register(builtin.NewRunCommand(
		builtin.WithDefaultTimeout(5),
		builtin.WithMaxOutput(50*1024),
	))

	// 创建 Agent，启用所有 Week 2 特性
	agent := core.NewAgent(
		core.NewRetryProvider(provider, retryPolicy),
		mem,
		registry,
		core.WithMaxTurns(10),
		core.WithLoopDetector(),          // 循环检测
		core.WithTracer(&jsonExporter{}), // Trace JSON 导出
		core.WithSummarizer(),            // 日志摘要
	)

	// 演示任务
	tasks := []string{
		"Hello, who are you?",
		"Read the file go.mod in the current directory",
		"List files in the current directory using run_command",
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	for i, task := range tasks {
		slog.Info("=== starting task", "index", i+1, "task", task)
		result, err := agent.Run(ctx, task)
		if err != nil {
			slog.Error("task failed", "index", i+1, "error", err)
			continue
		}
		slog.Info("=== task result", "index", i+1, "text", result.Text, "turns", result.Turns)
	}

	slog.Info("week2 demo complete. check for trace-*.json files in current directory")
}

// jsonExporter 将 trace 写入 trace-<id>.json 文件。
type jsonExporter struct{}

func (e *jsonExporter) Export(t *agoratrace.Trace) error {
	data, err := t.MarshalJSON()
	if err != nil {
		return err
	}
	return os.WriteFile("trace-"+t.TraceID+".json", data, 0644)
}

// mockProvider — 不需要真实 API key 时使用。
type mockProvider struct{ resp *llm.Response }

func (m *mockProvider) Chat(ctx context.Context, params llm.ChatParams) (*llm.Response, error) {
	return m.resp, nil
}
func (m *mockProvider) ChatStream(ctx context.Context, params llm.ChatParams, cb func(*llm.PartialResponse)) error {
	return llm.ErrStreamingUnsupported
}
func (m *mockProvider) Name() string                          { return "mock" }
func (m *mockProvider) EstimateTokens(msgs []llm.Message) int { return 0 }
