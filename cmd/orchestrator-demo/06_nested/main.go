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

// Nested Demo: Pipeline × Parallel 嵌套组合
// 内层：researcher → implementer（Pipeline）
// 内层：security-reviewer + perf-reviewer（Parallel）
// 外层：impl-pipeline → review-parallel（Pipeline）

func main() {
	// 1. 初始化 MiniMax provider
	provider, err := demo.NewMiniMaxProvider()
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
	tracer := demo.NewOrchestratorTracer("nested")

	// 2. 创建内层 Pipeline 的 Agent（researcher → implementer）
	researcherAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是研究员。搜索并整理用户认证系统的最佳实践。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)
	implementerAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是实现者。基于研究结果实现用户认证系统。输出完整的代码实现。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	// 3. 创建内层 Parallel 的 Agent（security + perf 审查）
	securityReviewerAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是安全审查员。审查代码中的安全漏洞。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)
	perfReviewerAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是性能审查员。审查代码的性能和扩展性问题。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	// 4. 包装为 Runner
	researcherRunner := orchestrator.NewAgentRunner("researcher", researcherAgent)
	implementerRunner := orchestrator.NewAgentRunner("implementer", implementerAgent)
	securityRunner := orchestrator.NewAgentRunner("security-reviewer", securityReviewerAgent)
	perfRunner := orchestrator.NewAgentRunner("perf-reviewer", perfReviewerAgent)

	// 5. 构建内层 Pipeline（researcher → implementer）
	implPipeline := orchestrator.NewOrchestrator("impl-pipeline",
		orchestrator.NewPipelineStrategizer(),
		[]orchestrator.Runner{researcherRunner, implementerRunner},
	)

	// 6. 构建内层 Parallel（security + perf 并发审查）
	reviewParallel := orchestrator.NewOrchestrator("review-parallel",
		orchestrator.NewParallelStrategizer(
			&orchestrator.ConcatMerger{Separator: "\n\n══════════════════════════════\n\n"},
			orchestrator.WithMaxWorkers(2),
		),
		[]orchestrator.Runner{securityRunner, perfRunner},
	)

	// 7. 构建外层 Pipeline（impl-pipeline → review-parallel）
	orch := orchestrator.NewOrchestrator("auth-system",
		orchestrator.NewPipelineStrategizer(),
		[]orchestrator.Runner{implPipeline, reviewParallel},
	)

	// 8. 执行
	ctx := context.Background()
	start := time.Now()
	result, err := orch.Run(ctx, "实现一个简单的用户认证系统")
	elapsed := time.Since(start)

	// 9. 输出
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Nested 完成 (%.1fs)\n", elapsed.Seconds())
	fmt.Printf("📥 Input tokens: %d  📤 Output tokens: %d\n",
		result.TotalInputTokens, result.TotalOutputTokens)
	fmt.Printf("\n📄 输出:\n%s\n", result.Text)
	fmt.Printf("\n🔍 Trace: traces/trace-%s.json\n", tracer.TraceID())
}
