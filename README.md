# Agora

Go Agent 框架。Module: `github.com/smrobot988-design/Agora`

## 是什么

Agora 是一个轻量级的 Go Agent 运行时引擎，抽象了 LLM Provider、Tool Registry、Memory 和路由决策。开发者只需关注prompt和工具注册，框架处理多轮对话、错误重试、循环检测和可观测性。

## 核心特性

- **多 Provider 支持** — 接入 Claude（Anthropic SDK）、MiniMax（OpenAI 兼容 API），通过 `Provider` 接口可扩展，正在支持更多的模型接入
- **流式输出** — 支持 Server-Sent Events 流式渲染，边生成边打印
- **内置工具** — `read_file`（文件读取）、`run_command`（shell 执行）
- **可观测性** — 内置了 RetryProvider（指数退避）、LoopDetector（循环检测）、Tracer（分布式追踪）、Summarizer（运行摘要）
- **上下文管理** — Memory + Trimmer 支持滑动窗口和 Token 预算控制

## 快速开始

### 安装

```bash
go get github.com/smrobot988-design/Agora
```

### 运行 Demo

```bash
# 编译
go build ./...

# 单元测试
go test ./...

# 交互式 CLI（支持 Claude 和 MiniMax）
export ANTHROPIC_API_KEY=your_key_here
go run ./cmd/agora/

# 流式 REPL（支持 tool use）
export MINIMAX_API_KEY=your_key_here
go run ./cmd/stream/ -provider minimax
```

### 5 分钟上手

```go
// 1. 创建 Provider（支持 Claude / MiniMax）
provider := llm.NewClaudeProvider(
    llm.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
    llm.WithModel(anthropic.ModelClaudeSonnet4_6),
)

// 2. 注册工具
registry := tool.NewRegistry()
registry.MustRegister(builtin.NewReadFile())
registry.MustRegister(builtin.NewRunCommand())

// 3. 创建 Agent
agent := core.NewAgent(provider,
    core.WithRegistry(registry),
    core.WithMaxTurns(20),
)

// 4. 运行
result, err := agent.Run(context.Background(), "帮我读取 go.mod 文件")
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.Text)
```

### 流式输出

```go
err := provider.ChatStream(ctx, llm.ChatParams{
    Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "写一首诗")},
}, func(pr *llm.PartialResponse) {
    if pr == nil {
        return // 流结束
    }
    switch pr.Type {
    case llm.StreamEventTextDelta:
        fmt.Print(pr.TextDelta) // 边生成边打印
    case llm.StreamEventToolDelta:
        // 收集 tool call delta
    case llm.StreamEventStop:
        fmt.Println() // 输出结束
    }
})
```

## 架构

```
pkg/
├── schema/          # 核心类型（ToolDefinition, ToolCall），零依赖
├── llm/             # Provider 接口 + Claude / MiniMax 实现
│   └── stream.go    # 流式事件类型（PartialResponse, StreamEvent*）
├── memory/
│   ├── store/       # 消息存储接口 + 内存实现
│   └── trimmer/     # 上下文截断策略（NoOp / SlidingWindow / TokenBudget）
├── tool/            # Tool 接口 + Func 便捷类型 + Registry
│   └── builtin/     # 内置工具：read_file, run_command
└── core/            # Agent 引擎
    ├── agent.go     # Agent 主入口
    ├── loop.go      # 多轮对话循环
    ├── router.go    # 路由决策（final / tool_call / error）
    ├── retry.go     # RetryProvider：指数退避 + jitter 重试
    ├── loop_detector.go  # LoopDetector：SHA256 指纹检测重复调用
    ├── tracer.go    # Tracer：Span 记录 + JSON Exporter
    ├── summary.go   # Summarizer：RunSummary 结构化日志
    └── trace/       # Trace/Span 类型 + Exporter 接口
cmd/
├── agora/           # 交互式多轮对话 CLI
├── example/        # Agent.Run 完整示例
├── stream/         # 流式 REPL（支持 tool use）
└── week2_demo/     # Week2 功能演示（Retry / Loop / Tracer / Summary）
```

## Provider 接口

```go
type Provider interface {
    Chat(ctx context.Context, params ChatParams) (*Response, error)
    ChatStream(ctx context.Context, params ChatParams, cb func(*PartialResponse)) error
    Name() string
    EstimateTokens(messages []Message) int
}
```

所有 Provider 实现（Claude、MiniMax）均实现此接口，可自由替换。

## 可观测性（可选启用）

```go
agent := core.NewAgent(provider,
    core.WithRegistry(registry),
    core.WithTracer(core.NewTracer("agent", &trace.JSONExporter{Dir: "./traces"})),
    core.WithLoopDetector(&core.LoopDetector{MaxRepeats: 3}),
    core.WithSummarizer(slog.Default(), slog.LevelInfo),
    core.WithRetryProvider(provider, &core.RetryPolicy{
        InitialDelay:   1 * time.Second,
        BackoffMultiplier: 2.0,
        Jitter:         0.2,
        MaxDelay:       30 * time.Second,
        MaxAttempts:    5,
        IsRetryable:    core.DefaultIsRetryable,
    }),
)
```

- **RetryProvider** — LLM 调用失败时自动指数退避重试
- **LoopDetector** — 检测重复工具调用和连续相同错误，防止死循环
- **Tracer** — 记录每个 LLM 调用和工具执行的起止时间，输出 JSON 文件
- **Summarizer** — 每次 Run 结束后输出结构化摘要日志

## 接下来计划
1. 支持多 agent 编排
2. 支持更多的模型产商

## License

MIT
