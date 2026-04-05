package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/smrobot988-design/Agora/pkg/llm"

	demo "github.com/smrobot988-design/Agora/cmd/orchestrator-demo/internal"
)

// Pipeline Demo: 研究员 → 写作者 → 编辑
// 演示顺序执行的流水线模式，每阶段输出作为下一阶段输入
// 每个 Agent 的输出以流式方式显示

func main() {
	// 1. 初始化 MiniMax provider
	provider, err := demo.NewMiniMaxProvider()
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
	tracer := demo.NewOrchestratorTracer("pipeline")

	ctx := context.Background()
	start := time.Now()

	// 2. 第一阶段：研究员
	fmt.Println("──────────────────────────────────────")
	fmt.Println("🔍 研究员")
	fmt.Println("──────────────────────────────────────")
	researchText, inputTokens, outputTokens, err := streamAgent(ctx, provider,
		"你是一个研究员，负责搜集和整理信息。输出结构化的研究报告。",
		"写一篇关于 Go 并发模型的技术博客")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 研究员 Error: %v\n", err)
		os.Exit(1)
	}
	totalIn, totalOut := inputTokens, outputTokens

	// 3. 第二阶段：写作者
	fmt.Println("\n──────────────────────────────────────")
	fmt.Println("✍️  写作者")
	fmt.Println("──────────────────────────────────────")
	writerText, inputTokens, outputTokens, err := streamAgent(ctx, provider,
		"你是写作专家。基于提供的研究材料撰写文章。要求：内容完整、结构清晰、语言流畅。",
		researchText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 写作者 Error: %v\n", err)
		os.Exit(1)
	}
	totalIn += inputTokens
	totalOut += outputTokens

	// 4. 第三阶段：编辑
	fmt.Println("\n──────────────────────────────────────")
	fmt.Println("📝 编辑")
	fmt.Println("──────────────────────────────────────")
	editorText, inputTokens, outputTokens, err := streamAgent(ctx, provider,
		"你是编辑，负责润色和优化文章。输出最终稿，确保文章专业且易读。",
		writerText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 编辑 Error: %v\n", err)
		os.Exit(1)
	}
	totalIn += inputTokens
	totalOut += outputTokens
	_ = editorText // 最终输出

	elapsed := time.Since(start)

	// 5. 输出摘要
	fmt.Println("\n──────────────────────────────────────")
	fmt.Printf("✅ Pipeline 完成 (%.1fs)\n", elapsed.Seconds())
	fmt.Printf("📥 Input tokens: %d  📤 Output tokens: %d\n", totalIn, totalOut)
	fmt.Printf("\n🔍 Trace: traces/trace-%s.json\n", tracer.TraceID())
}

// streamAgent 调用 provider.ChatStream 流式输出，返回完整文本和 token 统计
func streamAgent(ctx context.Context, provider *llm.MiniMaxProvider, systemPrompt, userInput string) (string, int, int, error) {
	var textDelta string
	var totalInput, totalOutput int

	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, userInput),
	}
	inputTokens := provider.EstimateTokens(messages)

	err := provider.ChatStream(ctx, llm.ChatParams{
		System:   systemPrompt,
		Messages: messages,
	}, func(pr *llm.PartialResponse) {
		if pr == nil {
			return
		}
		switch pr.Type {
		case llm.StreamEventTextDelta:
			fmt.Print(pr.TextDelta)
			textDelta += pr.TextDelta
		case llm.StreamEventStop:
			outputTokens := provider.EstimateTokens([]llm.Message{
				llm.NewTextMessage(llm.RoleAssistant, textDelta),
			})
			totalInput = inputTokens
			totalOutput = outputTokens
		}
	})
	if err != nil {
		return "", 0, 0, err
	}

	// 防止 stop 事件没触发，手动估算
	if totalOutput == 0 {
		totalInput = inputTokens
		totalOutput = provider.EstimateTokens([]llm.Message{
			llm.NewTextMessage(llm.RoleAssistant, textDelta),
		})
	}

	return textDelta, totalInput, totalOutput, nil
}
