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

// Debate Demo: 三专家多轮辩论，裁判综合结论
// 演示 DebateStrategizer：每轮让各专家看到他人观点后修正，最终由裁判综合

func main() {
	// 1. 初始化 MiniMax provider
	provider, err := demo.NewMiniMaxProvider()
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
	tracer := demo.NewOrchestratorTracer("debate")

	// 2. 创建三个专家 Agent
	perfAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是性能架构师。从性能角度评估技术方案。"+
				"如果看到其他专家的意见，请回应并修正你的观点。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)
	maintAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是可维护性专家。从代码可维护性角度评估方案。"+
				"如果看到其他专家的意见，请回应并修正你的观点。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)
	secAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是安全专家。从安全角度评估方案。"+
				"如果看到其他专家的意见，请回应并修正你的观点。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	// 3. 创建裁判 Agent（用于最终 Merge）
	judgeAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是技术总监。综合所有专家的观点，给出最终技术决策。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	// 4. 包装为 Runner
	perfRunner := orchestrator.NewAgentRunner("perf-expert", perfAgent)
	maintRunner := orchestrator.NewAgentRunner("maint-expert", maintAgent)
	secRunner := orchestrator.NewAgentRunner("sec-expert", secAgent)
	judgeRunner := orchestrator.NewAgentRunner("judge", judgeAgent)

	// 5. 创建 Debate Orchestrator
	orch := orchestrator.NewOrchestrator("tech-debate",
		orchestrator.NewDebateStrategizer(
			3, // 3 轮辩论
			&orchestrator.LLMMerger{
				Runner:         judgeRunner,
				PromptTemplate: "以下是各专家经过 %d 轮辩论后的最终观点：\n%s\n请综合给出技术决策。",
			},
		),
		[]orchestrator.Runner{perfRunner, maintRunner, secRunner},
	)

	// 6. 执行
	ctx := context.Background()
	start := time.Now()
	result, err := orch.Run(ctx, "我们应该用 sync.Mutex 还是 channel 来保护 Go 中的共享状态？")
	elapsed := time.Since(start)

	// 7. 输出
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Debate 完成 (%.1fs)\n", elapsed.Seconds())
	fmt.Printf("📥 Input tokens: %d  📤 Output tokens: %d\n",
		result.TotalInputTokens, result.TotalOutputTokens)
	fmt.Printf("\n📄 输出:\n%s\n", result.Text)
	fmt.Printf("\n🔍 Trace: traces/trace-%s.json\n", tracer.TraceID())
}
