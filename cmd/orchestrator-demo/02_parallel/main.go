package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/core/trace"
	"github.com/smrobot988-design/Agora/pkg/orchestrator"
	"github.com/smrobot988-design/Agora/pkg/tool"

	demo "github.com/smrobot988-design/Agora/cmd/orchestrator-demo/internal"
)

// Parallel Demo: 安全 + 性能 + 代码风格审查（并发执行）
// 演示多 Agent 并行扇出模式，用 ConcatMerger 拼接结果

func main() {
	// 1. 初始化 MiniMax provider
	provider, err := demo.NewMiniMaxProvider()
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
	tracer := demo.NewOrchestratorTracer("parallel")

	code := `package main

import "fmt"

func main() {
	data := make([]int, 1000000)
	for i := range data {
		data[i] = i * 2
	}
	fmt.Println(sum(data))
}

func sum(arr []int) int {
	total := 0
	for _, v := range arr {
		total += v
	}
	return total
}`

	// 2. 创建三个审查 Agent（不同视角）
	securityAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是安全专家。只关注代码中的安全隐患，如：整数溢出、空指针、SQL注入等。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)
	perfAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是性能专家。只关注性能瓶颈和优化建议，如：复杂度分析、内存占用、并发潜力等。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)
	styleAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是代码规范专家。只关注代码风格和最佳实践，如：命名规范、错误处理、文档注释等。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	// 3. 包装为 Runner
	securityRunner := orchestrator.NewAgentRunner("security", securityAgent)
	perfRunner := orchestrator.NewAgentRunner("performance", perfAgent)
	styleRunner := orchestrator.NewAgentRunner("style", styleAgent)

	// 4. 创建 Parallel Orchestrator（3 个并发 worker）
	orch := orchestrator.NewOrchestrator("code-review",
		orchestrator.NewParallelStrategizer(
			&orchestrator.ConcatMerger{Separator: "\n\n══════════════════════════════\n\n"},
			orchestrator.WithMaxWorkers(3),
		),
		[]orchestrator.Runner{securityRunner, perfRunner, styleRunner},
	)

	// 5. 执行
	ctx := context.Background()
	start := time.Now()
	result, err := orch.Run(ctx, "请审查以下 Go 代码：\n"+code)
	elapsed := time.Since(start)

	// 6. 输出
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Parallel 完成 (%.1fs)\n", elapsed.Seconds())
	fmt.Printf("📥 Input tokens: %d  📤 Output tokens: %d\n",
		result.TotalInputTokens, result.TotalOutputTokens)
	fmt.Printf("\n📄 输出:\n%s\n", result.Text)
	fmt.Printf("\n🔍 Trace: traces/trace-%s.json\n", tracer.TraceID())
}
