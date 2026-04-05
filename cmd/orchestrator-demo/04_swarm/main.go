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

// Swarm Demo: 客服系统交接模式
// 前台 → 技术支持 → 退款专员，通过 HandoffTool 转移控制权

func main() {
	// 1. 初始化 MiniMax provider
	provider, err := demo.NewMiniMaxProvider()
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
	tracer := demo.NewOrchestratorTracer("swarm")

	// 可用 Agent 列表（用于 HandoffTool 的 enum）
	availableAgents := []string{"front_desk", "tech_support", "refund_specialist"}

	// 2. 创建 HandoffTool（每个 Agent 都持有同一个 HandoffTool 实例）
	handoffTool := orchestrator.NewHandoffTool(availableAgents)

	// 3. 创建各角色 Agent（各自持有 HandoffTool）
	frontDeskAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是前台客服。处理简单问题，复杂问题交接给专家。"+
				"如果遇到技术问题，使用 handoff 工具转给 tech_support。"+
				"如果遇到退款问题，使用 handoff 工具转给 refund_specialist。",
		)),
		func() *tool.Registry {
			r := tool.NewRegistry()
			r.Register(handoffTool)
			return r
		}(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	techSupportAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是技术支持专家。解决技术问题。"+
				"如果问题已解决但需要退款，可以交接给 refund_specialist。",
		)),
		func() *tool.Registry {
			r := tool.NewRegistry()
			r.Register(handoffTool)
			return r
		}(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	refundAgent := core.NewAgent(provider,
		core.NewMemory(core.WithSystemPrompt(
			"你是退款专员。处理退款申请并给出最终结果。",
		)),
		func() *tool.Registry {
			r := tool.NewRegistry()
			r.Register(handoffTool)
			return r
		}(),
		core.WithTracer(&trace.JSONExporter{Dir: "traces"}),
	)

	// 4. 包装为 Runner
	frontDeskRunner := orchestrator.NewAgentRunner("front_desk", frontDeskAgent)
	techSupportRunner := orchestrator.NewAgentRunner("tech_support", techSupportAgent)
	refundRunner := orchestrator.NewAgentRunner("refund_specialist", refundAgent)

	// 5. 创建 SwarmRunner
	swarm := orchestrator.NewSwarmRunner("customer-service",
		"front_desk", // 入口 Agent
		map[string]orchestrator.Runner{
			"front_desk":       frontDeskRunner,
			"tech_support":     techSupportRunner,
			"refund_specialist": refundRunner,
		},
		orchestrator.WithMaxHandoffs(10),
	)

	// 6. 执行
	ctx := context.Background()
	start := time.Now()
	result, err := swarm.Run(ctx, "你好，我昨晚下的订单到现在还没到，而且我最近失业了想退款")
	elapsed := time.Since(start)

	// 7. 输出
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Swarm 完成 (%.1fs)\n", elapsed.Seconds())
	fmt.Printf("📥 Input tokens: %d  📤 Output tokens: %d\n",
		result.TotalInputTokens, result.TotalOutputTokens)
	fmt.Printf("\n📄 输出:\n%s\n", result.Text)
	fmt.Printf("\n🔍 Trace: traces/trace-%s.json\n", tracer.TraceID())
}
