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

// Supervisor Demo: 指挥官模式，子 Agent 以工具形式注册
// 演示 LLM 自主调度子 Agent 的模式，Supervisor 自己也是 Agent

func main() {
	// 1. 初始化 MiniMax provider
	provider, err := demo.NewMiniMaxProvider()
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
	tracer := demo.NewOrchestratorTracer("supervisor")

	// 2. 创建子 Agent（研究员、程序员、测试员）
	researcherAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是研究员，搜索和整理信息。输出结构化的研究结果。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)
	coderAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是程序员，编写和调试代码。输出完整的代码实现。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)
	testerAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是测试工程师，编写测试用例。输出测试代码和测试结果。",
		)),
		tool.NewRegistry(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	// 3. 构建 Supervisor 的 Tool Registry（子 Agent 作为工具注册）
	supervisorRegistry := tool.NewRegistry()
	supervisorRegistry.Register(orchestrator.NewAgentTool(
		"researcher",
		"搜索和整理信息。当你需要了解某个主题的背景时调用。",
		orchestrator.NewAgentRunner("researcher", researcherAgent),
	))
	supervisorRegistry.Register(orchestrator.NewAgentTool(
		"coder",
		"编写和调试代码。当你需要实现功能或修复 bug 时调用。",
		orchestrator.NewAgentRunner("coder", coderAgent),
	))
	supervisorRegistry.Register(orchestrator.NewAgentTool(
		"tester",
		"编写测试用例。当你需要验证代码正确性时调用。",
		orchestrator.NewAgentRunner("tester", testerAgent),
	))

	// 4. 创建 Supervisor Agent
	supervisorAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是项目经理。分析用户需求，调用 researcher、coder、tester 完成任务。"+
				"你可以多次调用子 Agent，直到任务完成。综合子 Agent 的结果给出最终回答。",
		)),
		supervisorRegistry,
		core.WithMaxTurns(20),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	// 5. 创建 Supervisor Strategizer
	supervisorRunner := orchestrator.NewAgentRunner("supervisor", supervisorAgent)
	orch := orchestrator.NewOrchestrator("http-client-supervisor",
		orchestrator.NewSupervisorStrategizer(supervisorRunner),
		nil, // SupervisorStrategizer 不使用 runners slice
	)

	// 6. 执行
	ctx := context.Background()
	start := time.Now()
	result, err := orch.Run(ctx, "用 Go 实现一个支持超时和重试的 HTTP 客户端")
	elapsed := time.Since(start)

	// 7. 输出
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Supervisor 完成 (%.1fs)\n", elapsed.Seconds())
	fmt.Printf("📥 Input tokens: %d  📤 Output tokens: %d\n",
		result.TotalInputTokens, result.TotalOutputTokens)
	fmt.Printf("\n📄 输出:\n%s\n", result.Text)
	fmt.Printf("\n🔍 Trace: traces/trace-%s.json\n", tracer.TraceID())
}
