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

所有入口支持通过命令行 flag、环境变量或 `.local.env` 配置 API key：

```bash
# 基础用法（纯净 Agent，无 trace/retry/loop）
go run ./cmd/example/                          # 默认 claude
go run ./cmd/example/ -provider minimax

# Week 2 全特性（Retry + Loop + Trace + Summary）
go run ./cmd/with-trace/
go run ./cmd/with-trace/ -provider minimax

# 流式 REPL（边生成边打印）
go run ./cmd/stream/ -provider minimax
go run ./cmd/stream/ -provider claude

# 支持以下配置方式（优先级：flag > env > .local.env）：

# 方式 1：环境变量
export ANTHROPIC_API_KEY=your_key
export ANTHROPIC_BASE_URL=https://api.ccodezh.com  # claude 第三方代理
export MINIMAX_API_KEY=your_key

# 方式 2：.local.env 文件（不会被 git 提交）
echo "ANTHROPIC_API_KEY=sk-xxx" > .local.env
echo "MINIMAX_API_KEY=sk-xxx" >> .local.env

# 方式 3：命令行 flag
go run ./cmd/example/ -api-key=sk-xxx -base-url=https://api.ccodezh.com
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
registry.Register(builtin.NewReadFile())
registry.Register(builtin.NewRunCommand())

// 3. 创建 Memory
mem := core.NewMemory(core.WithSystemPrompt("You are a helpful assistant."))

// 4. 创建 Agent
agent := core.NewAgent(provider, mem, registry, core.WithMaxTurns(20))

// 5. 运行
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
├── core/            # Agent 引擎
│   ├── agent.go     # Agent 主入口
│   ├── loop.go      # 多轮对话循环
│   ├── router.go    # 路由决策（final / tool_call / error）
│   ├── retry.go     # RetryProvider：指数退避 + jitter 重试
│   ├── loop_detector.go  # LoopDetector：SHA256 指纹检测重复调用
│   ├── tracer.go    # Tracer：Span 记录 + JSON Exporter
│   ├── summary.go   # Summarizer：RunSummary 结构化日志
│   └── trace/       # Trace/Span 类型 + Exporter 接口
└── orchestrator/    # 多 Agent 编排
    ├── runner.go        # Runner 接口 + AgentRunner 适配器
    ├── strategizer.go   # Strategizer 接口 + Orchestrator
    ├── agent_tool.go    # AgentTool（Runner → tool.Tool 适配器）
    ├── pipeline.go      # PipelineStrategizer
    ├── parallel.go      # ParallelStrategizer
    ├── supervisor.go    # SupervisorStrategizer
    ├── swarm.go         # SwarmRunner + HandoffTool
    ├── debate.go        # DebateStrategizer + DebateFormatter
    └── trace.go         # OrchestratorTracer（跨 Agent 追踪）
cmd/
├── agora/           # 交互式多轮对话 CLI
├── example/        # Agent.Run 完整示例
├── stream/         # 流式 REPL（支持 tool use）
├── with-trace/     # 全特性演示（Retry / Loop / Tracer / Summary）
└── orchestrator-demo/  # 多 Agent 编排模式演示
    ├── 01_pipeline/     # 流水线：研究员 → 写作者 → 编辑
    ├── 02_parallel/     # 并行：安全 + 性能 + 风格审查
    ├── 03_supervisor/   # 指挥官：子 Agent 以工具注册
    ├── 04_swarm/        # 群体交接：前台 → 技术 → 退款
    ├── 05_debate/       # 辩论：三专家多轮 + 裁判综合
    └── 06_nested/       # 嵌套：Pipeline × Parallel 组合
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

## 多 Agent 编排模式

Agora 支持 6 种编排模式，适用于不同场景：

### 1. Pipeline（流水线）

顺序执行多阶段任务，每阶段输出作为下一阶段输入。

```bash
go run ./cmd/orchestrator-demo/01_pipeline/
```

```go
// 研究员 → 写作者 → 编辑
orch := orchestrator.NewOrchestrator("content-pipeline",
    orchestrator.NewPipelineStrategizer(),
    []orchestrator.Runner{researcherRunner, writerRunner, editorRunner},
)
result, _ := orch.Run(ctx, "写一篇关于 Go 并发的技术博客")
```

**适用场景**：内容创作（研究→写作→编辑）、代码生成（需求分析→实现→审查）

---

### 2. Parallel（并行）

多个 Agent 并发执行同一任务，用 Merger 合并结果。

```bash
go run ./cmd/orchestrator-demo/02_parallel/
```

```go
// 安全 + 性能 + 风格并发审查
orch := orchestrator.NewOrchestrator("code-review",
    orchestrator.NewParallelStrategizer(
        &orchestrator.ConcatMerger{Separator: "\n\n---\n\n"},
        orchestrator.WithMaxWorkers(3),
    ),
    []orchestrator.Runner{securityRunner, perfRunner, styleRunner},
)
```

**适用场景**：多维度审查（安全+性能+规范）、多语言翻译、多角度分析

---

### 3. Supervisor（指挥官）

一个 Supervisor Agent 自主决定何时调用哪个子 Agent，子 Agent 以工具形式注册。

```bash
go run ./cmd/orchestrator-demo/03_supervisor/
```

```go
// 子 Agent 注册到 supervisorRegistry
supervisorRegistry := tool.NewRegistry()
supervisorRegistry.Register(orchestrator.NewAgentTool(
    "researcher", "搜索和整理信息", researcherRunner,
))
supervisorRegistry.Register(orchestrator.NewAgentTool(
    "coder", "编写代码", coderRunner,
))

supervisorAgent := core.NewAgent(provider, mem, supervisorRegistry)
orch := orchestrator.NewOrchestrator("supervisor",
    orchestrator.NewSupervisorStrategizer(
        orchestrator.NewAgentRunner("supervisor", supervisorAgent),
    ), nil)
```

**适用场景**：任务分配（项目经理）、客服路由、复杂推理

---

### 4. Swarm（群体交接）

Agent 之间转移控制权，无需返回协调者。交接信号通过 HandoffTool 实现。

```bash
go run ./cmd/orchestrator-demo/04_swarm/
```

```go
// 前台 → 技术支持 → 退款专员
swarm := orchestrator.NewSwarmRunner("customer-service",
    "front_desk",
    map[string]orchestrator.Runner{
        "front_desk":       frontDeskRunner,
        "tech_support":     techSupportRunner,
        "refund_specialist": refundRunner,
    },
    orchestrator.WithMaxHandoffs(10),
)
```

**适用场景**：客服系统（前台→专家→退款）、多步骤流程（接单→处理→回访）

---

### 5. Debate（辩论）

多个 Agent 多轮辩论，每轮看到他人观点后修正立场，最终由裁判 Agent 综合结论。

```bash
go run ./cmd/orchestrator-demo/05_debate/
```

```go
// 三专家辩论 + 裁判综合
orch := orchestrator.NewOrchestrator("tech-debate",
    orchestrator.NewDebateStrategizer(
        3, // 3 轮辩论
        &orchestrator.LLMMerger{Runner: judgeRunner, PromptTemplate: "..."},
    ),
    []orchestrator.Runner{perfRunner, maintRunner, secRunner},
)
```

**适用场景**：技术决策（多角度权衡）、内容审核、复杂推理

---

### 6. Nested（嵌套）

Pipeline 和 Parallel 嵌套组合，实现复杂编排结构。

```bash
go run ./cmd/orchestrator-demo/06_nested/
```

```go
// 内层：researcher → implementer（Pipeline）
// 内层：security + perf 审查（Parallel）
// 外层：impl-pipeline → review-parallel（Pipeline）
implPipeline := orchestrator.NewOrchestrator("impl", pipelineStrategy, []Runner{...})
reviewParallel := orchestrator.NewOrchestrator("review", parallelStrategy, []Runner{...})
orch := orchestrator.NewOrchestrator("auth-system",
    orchestrator.NewPipelineStrategizer(),
    []orchestrator.Runner{implPipeline, reviewParallel},
)
```

**适用场景**：复杂系统（研究→实现→多维度审查）、企业流程自动化

---

### Trace 可视化

每个 Demo 运行时会在 `traces/` 目录生成 JSON trace 文件，用 `cmd/trace-viewer/index.html` 打开可查看执行详情：

```bash
# 运行 demo 后
open ./cmd/trace-viewer/index.html
# 选择 traces/trace-<id>.json 文件加载
```

## 接下来计划

1. 支持更多的模型产商
2. 扩展 Tool 系统（Tool 并发执行、Tool 异步调用）
3. 支持更多编排模式（A2A 协议、状态机编排）

## License

MIT
