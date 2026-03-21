package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

func main() {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	}

	provider := llm.NewClaudeProvider()
	fmt.Printf("Provider: %s\n", provider.Name())

	tools := []schema.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: schema.PropertySchema{
				Properties: map[string]interface{}{
					"city": map[string]interface{}{
						"type":        "string",
						"description": "The city name",
					},
				},
				Required: []string{"city"},
			},
		},
	}

	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "What's the weather in San Francisco?"),
	}

	ctx := context.Background()

	for turn := 1; turn <= 5; turn++ {
		fmt.Printf("\n--- Turn %d ---\n", turn)

		resp, err := provider.Chat(ctx, messages, tools)
		if err != nil {
			log.Fatalf("Chat error: %v", err)
		}

		fmt.Printf("Stop reason: %s\n", resp.StopReason)
		fmt.Printf("Tokens: %d in / %d out\n", resp.InputTokens, resp.OutputTokens)

		if resp.Text != "" {
			fmt.Printf("Text: %s\n", resp.Text)
		}

		if resp.StopReason == llm.StopReasonEndTurn {
			fmt.Println("\nDone.")
			break
		}

		if resp.StopReason == llm.StopReasonToolUse {
			// Build assistant message with tool_use blocks
			assistantBlocks := []llm.ContentBlock{}
			if resp.Text != "" {
				assistantBlocks = append(assistantBlocks, llm.ContentBlock{Type: llm.BlockText, Text: resp.Text})
			}
			for _, tc := range resp.ToolCalls {
				assistantBlocks = append(assistantBlocks, llm.ContentBlock{
					Type:     llm.BlockToolUse,
					ToolCall: tc,
				})
			}
			messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: assistantBlocks})

			// Execute tools (stubbed) and build results
			var results []llm.ToolResult
			for _, tc := range resp.ToolCalls {
				inputJSON, _ := json.Marshal(tc.Input)
				fmt.Printf("Tool call: %s(%s)\n", tc.Name, string(inputJSON))

				result := `{"temperature": "18°C", "condition": "partly cloudy", "humidity": "72%"}`
				results = append(results, llm.ToolResult{
					ToolCallID: tc.ID,
					Content:    result,
				})
			}
			messages = append(messages, llm.NewToolResultMessage(results))
		}
	}
}
