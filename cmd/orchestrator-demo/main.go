package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/orchestrator"
)

// patterns maps CLI pattern names to their builder.
var patterns = map[string]struct {
	desc      string
	input     string
	builder   func(io.Writer) orchestrator.Runner
}{
	"pipeline": {
		desc:   "内容创作流水线: 研究员 → 写作者 → 编辑",
		input:  "写一篇关于 Go 并发模型的技术博客",
		builder: buildPipelineDemo,
	},
	"parallel": {
		desc:   "代码审查: 安全 + 性能 + 代码风格 并行审查",
		input:  "请审查以下 Go 代码：\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n    data := make([]int, 1000000)\n    for i := range data {\n        data[i] = i * 2\n    }\n    fmt.Println(sum(data))\n}\n\nfunc sum(arr []int) int {\n    total := 0\n    for _, v := range arr {\n        total += v\n    }\n    return total\n}",
		builder: buildParallelDemo,
	},
	"supervisor": {
		desc:   "项目管理器: 动态决定调用哪个子 Agent（子 Agent 以工具形式注册）",
		input:  "用 Go 实现一个支持超时和重试的 HTTP 客户端",
		builder: buildSupervisorDemo,
	},
	"swarm": {
		desc:   "客服系统: 前台 → 技术支持 → 退款专员 动态交接",
		input:  "你好，我昨晚下的订单到现在还没到，而且我最近失业了想退款",
		builder: buildSwarmDemo,
	},
	"debate": {
		desc:   "技术辩论: 三个专家多轮辩论后由裁判综合结论",
		input:  "我们应该用 sync.Mutex 还是 channel 来保护 Go 中的共享状态？",
		builder: buildDebateDemo,
	},
	"nested": {
		desc:   "嵌套编排: Supervisor 协调 Pipeline（研究→实现）和 Parallel（安全审查）",
		input:  "实现一个简单的用户认证系统",
		builder: buildNestedDemo,
	},
}

// demoAgent simulates an LLM agent with predefined behavior.
// It implements orchestrator.Runner, so it works with all patterns.
type demoAgent struct {
	name    string
	prompt  string
	respond func(name, input string) string
}

func (a *demoAgent) Run(ctx context.Context, input string) (*core.Result, error) {
	fmt.Fprintf(os.Stderr, "  ▶ [%s] 收到: %s\n", a.name, trunc(input, 60))
	select {
	case <-time.After(400 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	output := a.respond(a.name, input)
	fmt.Fprintf(os.Stderr, "  ✓ [%s] 输出 (%d chars)\n", a.name, len(output))
	return &core.Result{
		Text:              output,
		TotalInputTokens:  estimateTokens(input),
		TotalOutputTokens: estimateTokens(output),
		Turns:             1,
	}, nil
}

func (a *demoAgent) Name() string { return a.name }

func estimateTokens(s string) int { return len(s) / 4 }
func trunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func main() {
	pattern := flag.String("pattern", "", "编排模式: "+strings.Join(keys(patterns), ", "))
	task := flag.String("task", "", "自定义任务（覆盖默认输入）")
	flag.Parse()

	if *pattern == "" {
		printUsage()
		return
	}

	p, ok := patterns[*pattern]
	if !ok {
		fmt.Printf("未知模式: %s\n\n", *pattern)
		printUsage()
		return
	}

	input := p.input
	if *task != "" {
		input = *task
	}

	fmt.Println("┌─────────────────────────────────────────────────────────────────┐")
	fmt.Printf("│ Agora 多 Agent 编排演示  —  %-10s                      │\n", *pattern)
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ %-63s │\n", p.desc)
	fmt.Println("└─────────────────────────────────────────────────────────────────┘")
	fmt.Printf("\n📥 输入:\n%s\n", trunc(input, 300))
	fmt.Println()

	ctx := context.Background()
	runner := p.builder(os.Stderr)

	fmt.Fprintf(os.Stderr, "\n▶ 启动编排...\n\n")
	start := time.Now()
	result, err := runner.Run(ctx, input)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("\n❌ 执行失败: %v\n", err)
		return
	}

	fmt.Println("\n┌─────────────────────────────────────────────────────────────────┐")
	fmt.Printf("│ 执行完成  ✅  (耗时 %s)                              │\n", elapsed.Round(100*time.Millisecond))
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ 📥 Token: 输入 %d / 输出 %d                                    │\n", result.TotalInputTokens, result.TotalOutputTokens)
	fmt.Println("├─────────────────────────────────────────────────────────────────┤")
	fmt.Println("│ 📤 输出:                                                       │")
	for _, line := range wrapBox(result.Text, 65) {
		fmt.Printf("│ %-65s │\n", line)
	}
	fmt.Println("└─────────────────────────────────────────────────────────────────┘")
}

func keys(m map[string]struct{ desc, input string; builder func(io.Writer) orchestrator.Runner }) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func printUsage() {
	fmt.Println("Agora 多 Agent 编排演示")
	fmt.Println("用法: go run ./cmd/orchestrator-demo/ -pattern=<模式> [-task=<任务>]")
	fmt.Println()
	fmt.Println("可用模式:")
	for name, p := range patterns {
		fmt.Printf("  %-10s  %s\n", name, p.desc)
	}
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  go run ./cmd/orchestrator-demo/ -pattern=pipeline")
	fmt.Println("  go run ./cmd/orchestrator-demo/ -pattern=supervisor -task=帮我写个计算器")
}

// wrapBox wraps text into lines that fit within a box of given width.
func wrapBox(text string, width int) []string {
	lines := strings.Split(text, "\n")
	var out []string
	for _, line := range lines {
		for len(line) > width {
			out = append(out, line[:width])
			line = line[width:]
		}
		if line != "" {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// =============================================================================
// Pattern Demos
// =============================================================================

func buildPipelineDemo(_ io.Writer) orchestrator.Runner {
	fmt.Fprintln(os.Stderr, "═══ Pipeline ═══  顺序执行: 研究员 → 写作者 → 编辑")

	researcher := &demoAgent{
		name:   "researcher",
		prompt: "研究员",
		respond: func(n, input string) string {
			return "【研究报告】Go 并发模型核心概念：\n" +
				"1. Goroutine — 轻量级线程，由 Go 运行时管理\n" +
				"2. Channel — 用于 goroutine 之间的通信和同步\n" +
				"3. sync 包 — Mutex、WaitGroup、Cond 等传统同步原语"
		},
	}

	writer := &demoAgent{
		name:   "writer",
		prompt: "写作者",
		respond: func(n, input string) string {
			return "# Go 并发模型详解\n\n" +
				"Go 语言以其独特的并发模型著称，通过 goroutine 和 channel " +
				"提供了一种优雅的并发编程方式。"
		},
	}

	editor := &demoAgent{
		name:   "editor",
		prompt: "编辑",
		respond: func(n, input string) string {
			return "# Go 并发模型详解\n\n" +
				"Go 语言以其独特的并发模型著称，通过 goroutine 和 channel " +
				"提供了一种优雅的并发编程方式。\n\n" +
				"## 核心概念\n\n" +
				"1. **Goroutine** — 轻量级执行单元\n" +
				"2. **Channel** — 类型安全的通信机制\n" +
				"3. **sync 包** — 传统同步原语\n\n" +
				"## 最佳实践\n" +
				"- 优先使用 channel 实现优雅的流程控制\n" +
				"- mutex 用于保护共享数据结构"
		},
	}

	return orchestrator.NewOrchestrator("content-pipeline",
		orchestrator.NewPipelineStrategizer(),
		[]orchestrator.Runner{researcher, writer, editor},
	)
}

func buildParallelDemo(_ io.Writer) orchestrator.Runner {
	fmt.Fprintln(os.Stderr, "═══ Parallel ═══  并发执行: 安全 + 性能 + 代码风格 同时审查")

	security := &demoAgent{
		name:   "security",
		prompt: "安全专家",
		respond: func(n, input string) string {
			return "【安全分析】\n" +
				"1. 整数溢出风险：i*2 在 i>INT_MAX/2 时溢出\n" +
				"2. 建议：使用 math.Mul64 或类型检查\n" +
				"3. 内存占用 ~8MB，当前可接受"
		},
	}

	performance := &demoAgent{
		name:   "performance",
		prompt: "性能专家",
		respond: func(n, input string) string {
			return "【性能分析】\n" +
				"1. O(n) 复杂度，无法进一步优化\n" +
				"2. for-range 优于索引遍历（Go 编译器优化更好）\n" +
				"3. 百万级数据无 JIT，优化空间有限\n" +
				"4. 并行化潜力：可考虑 worker pool"
		},
	}

	style := &demoAgent{
		name:   "style",
		prompt: "代码规范专家",
		respond: func(n, input string) string {
			return "【代码风格】\n" +
				"1. 缺少错误处理\n" +
				"2. 建议添加单元测试\n" +
				"3. sum 函数名太通用\n" +
				"4. 缺少注释和文档字符串"
		},
	}

	return orchestrator.NewOrchestrator("code-review",
		orchestrator.NewParallelStrategizer(
			&orchestrator.ConcatMerger{Separator: "\n\n════\n\n"},
		),
		[]orchestrator.Runner{security, performance, style},
	)
}

func buildSupervisorDemo(_ io.Writer) orchestrator.Runner {
	fmt.Fprintln(os.Stderr, "═══ Supervisor ═══  指挥官模式: 子 Agent 以工具形式注册，LLM 自主调度")

	// Each sub-agent is a Runner.
	researcher := &demoAgent{
		name:   "researcher",
		prompt: "研究员",
		respond: func(n, input string) string {
			return "HTTP 限流常用算法：\n" +
				"1. 令牌桶(Token Bucket) — 允许突发流量\n" +
				"2. 滑动窗口 — 平滑限流\n" +
				"3. 固定窗口计数器 — 实现简单但有边界突刺"
		},
	}

	coder := &demoAgent{
		name:   "coder",
		prompt: "程序员",
		respond: func(n, input string) string {
			return "package middleware\n\n" +
				"type RateLimiter struct {\n" +
				"    rate   float64\n" +
				"    burst  int\n" +
				"    tokens float64\n" +
				"    mu     sync.Mutex\n" +
				"}\n\n" +
				"func (rl *RateLimiter) Allow() bool {\n" +
				"    rl.mu.Lock()\n" +
				"    defer rl.mu.Unlock()\n" +
				"    // ... 令牌桶实现\n" +
				"}"
		},
	}

	tester := &demoAgent{
		name:   "tester",
		prompt: "测试工程师",
		respond: func(n, input string) string {
			return "测试覆盖：\n" +
				"- TestRateLimiter_Burst 突发流量测试\n" +
				"- TestRateLimiter_Refill 令牌补充测试\n" +
				"- TestRateLimiter_Concurrent 并发安全测试"
		},
	}

	// The supervisor orchestrates by deciding when to call which sub-agent.
	// In this demo, we simulate what the supervisor does:
	// 1. Call researcher → get algorithms
	// 2. Call coder → implement
	// 3. Call tester → write tests
	// 4. Return final summary
	supervisor := &demoAgent{
		name:   "supervisor",
		prompt: "项目经理",
		respond: func(n, input string) string {
			// Simulate supervisor calling sub-agents in sequence.
			algo := researcher.respond("researcher", input)
			impl := coder.respond("coder", input)
			tests := tester.respond("tester", input)
			return "✅ HTTP 限流客户端实现完成\n\n" +
				"【算法选择】" + algo[:50] + "...\n\n" +
				"【实现】\n" + impl + "\n\n" +
				"【测试】\n" + tests
		},
	}

	return orchestrator.NewOrchestrator("http-client-supervisor",
		orchestrator.NewSupervisorStrategizer(supervisor),
		nil, // runners unused by supervisor (tools are in its registry)
	)
}

func buildSwarmDemo(_ io.Writer) orchestrator.Runner {
	fmt.Fprintln(os.Stderr, "═══ Swarm ═══  交接模式: Agent 之间转移控制权，无需返回协调者")

	frontDesk := &demoAgent{
		name:   "front_desk",
		prompt: "前台客服",
		respond: func(n, input string) string {
			// Detect that this needs technical + refund support.
			return "HANDOFF:tech_support:用户反馈订单未到，且要求退款。请先处理技术问题。"
		},
	}

	techSupport := &demoAgent{
		name:   "tech_support",
		prompt: "技术支持",
		respond: func(n, input string) string {
			s := "已查询订单：快递因地址不详延迟派送，预计明天送达。" +
				"技术问题已确认解决，现在交接给退款专员。"
			return "HANDOFF:refund_specialist:" + s
		},
	}

	refundSpecialist := &demoAgent{
		name:   "refund_specialist",
		prompt: "退款专员",
		respond: func(n, input string) string {
			return "【退款处理完成】\n" +
				"- 订单金额：299 元\n" +
				"- 退款方式：原路返回\n" +
				"- 预计到账：3-5 个工作日\n\n" +
				"感谢您的来电，祝您生活愉快！"
		},
	}

	return orchestrator.NewSwarmRunner("customer-service",
		"front_desk",
		map[string]orchestrator.Runner{
			"front_desk":       frontDesk,
			"tech_support":      techSupport,
			"refund_specialist": refundSpecialist,
		},
	)
}

func buildDebateDemo(_ io.Writer) orchestrator.Runner {
	fmt.Fprintln(os.Stderr, "═══ Debate ═══  多轮辩论: 三专家各抒己见，裁判综合")

	perf := &demoAgent{
		name:   "perf-expert",
		prompt: "性能专家",
		respond: func(n, input string) string {
			return "Mutex 在低竞争场景下性能更优，开销约 50ns。" +
				"Channel 需要堆分配，在高吞吐量时有额外开销。"
		},
	}

	maint := &demoAgent{
		name:   "maint-expert",
		prompt: "可维护性专家",
		respond: func(n, input string) string {
			return "Channel 是 Go 的惯用法，符合 Go 的设计哲学。" +
				"通过 channel 通信使并发逻辑更清晰，死锁风险更低。"
		},
	}

	security := &demoAgent{
		name:   "sec-expert",
		prompt: "安全专家",
		respond: func(n, input string) string {
			return "Mutex 使用不当时风险更高（忘记解锁、死锁）。" +
				"Channel 的阻塞语义使问题更容易在测试阶段暴露。"
		},
	}

	judge := &demoAgent{
		name:   "judge",
		prompt: "技术总监",
		respond: func(n, input string) string {
			return "【最终决策】\n\n" +
				"推荐：**优先使用 Channel，必要时使用 Mutex**\n\n" +
				"理由：\n" +
				"1. Go 惯用法优先 — channel 是核心特性\n" +
				"2. 简单通信用 channel — 协程间通信和流程控制\n" +
				"3. 性能关键代码段用 Mutex — 计数器等细粒度保护\n" +
				"4. 避免混合使用 — 同一数据结构只选一种保护方式"
		},
	}

	return orchestrator.NewOrchestrator("tech-debate",
		orchestrator.NewDebateStrategizer(
			2, // 2 debate rounds
			&conciseMerger{runner: judge},
		),
		[]orchestrator.Runner{perf, maint, security},
	)
}

func buildNestedDemo(_ io.Writer) orchestrator.Runner {
	fmt.Fprintln(os.Stderr, "═══ Nested ═══  嵌套编排: Pipeline × Parallel 组合")

	// Inner pipeline: research → implement
	researcher := &demoAgent{
		name:   "researcher",
		prompt: "研究员",
		respond: func(n, in string) string {
			return "用户认证最佳实践：\n" +
				"1. 密码用 bcrypt 哈希存储\n" +
				"2. JWT token 认证\n" +
				"3. 安全的 session 管理"
		},
	}

	implementer := &demoAgent{
		name:   "implementer",
		prompt: "实现者",
		respond: func(n, in string) string {
			return "已实现基础认证系统：\n" +
				"- Register / Login API\n" +
				"- bcrypt 密码哈希\n" +
				"- JWT token 生成"
		},
	}

	implPipeline := orchestrator.NewOrchestrator("impl",
		orchestrator.NewPipelineStrategizer(),
		[]orchestrator.Runner{researcher, implementer},
	)

	// Inner parallel: code review
	securityReview := &demoAgent{
		name:   "security-reviewer",
		prompt: "安全审查员",
		respond: func(n, in string) string {
			return "安全审查通过：使用了 bcrypt，建议添加登录失败次数限制。"
		},
	}

	perfReview := &demoAgent{
		name:   "perf-reviewer",
		prompt: "性能审查员",
		respond: func(n, in string) string {
			return "性能审查：JWT 验证 O(1)，bcrypt cost=12 适中。"
		},
	}

	review := orchestrator.NewOrchestrator("review",
		orchestrator.NewParallelStrategizer(
			&orchestrator.ConcatMerger{Separator: "\n---\n"},
		),
		[]orchestrator.Runner{securityReview, perfReview},
	)

	// Outer: pipeline of impl-pipeline → review
	return orchestrator.NewOrchestrator("auth-system",
		orchestrator.NewPipelineStrategizer(),
		[]orchestrator.Runner{implPipeline, review},
	)
}

// conciseMerger synthesizes multiple results by invoking a judge runner.
// This is how LLM-based result merging works (like LLMMerger in production).
type conciseMerger struct {
	runner orchestrator.Runner
}

func (m *conciseMerger) Merge(ctx context.Context, results []*core.Result) (*core.Result, error) {
	if len(results) == 0 {
		return &core.Result{}, nil
	}

	// Build a summary prompt for the judge.
	var parts []string
	totalIn, totalOut := 0, 0
	for _, r := range results {
		parts = append(parts, r.Text)
		totalIn += r.TotalInputTokens
		totalOut += r.TotalOutputTokens
	}

	synthesis := "各位专家的观点如下：\n\n"
	for i, p := range parts {
		synthesis += fmt.Sprintf("%d. %s\n\n", i+1, p)
	}
	synthesis += "请综合以上观点，给出最终技术决策。"

	synthResult, err := m.runner.Run(context.Background(), synthesis)
	if err != nil {
		return nil, fmt.Errorf("judge synthesis: %w", err)
	}

	synthResult.TotalInputTokens += totalIn
	synthResult.TotalOutputTokens += totalOut
	return synthResult, nil
}
