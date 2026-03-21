package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/memory"
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

	// Use Memory to manage conversation history
	mem := memory.New(
		memory.WithSystemPrompt("You are a helpful weather assistant. Answer concisely."),
	)
	if err := mem.AddUserMessage("What's the weather in San Francisco?"); err != nil {
		log.Fatalf("AddUserMessage error: %v", err)
	}

	ctx := context.Background()

	for turn := 1; turn <= 5; turn++ {
		fmt.Printf("\n--- Turn %d ---\n", turn)

		msgs, err := mem.Messages()
		if err != nil {
			log.Fatalf("Messages error: %v", err)
		}

		resp, err := provider.Chat(ctx, llm.ChatParams{
			System:   mem.SystemPrompt(),
			Messages: msgs,
			Tools:    tools,
		})
		if err != nil {
			log.Fatalf("Chat error: %v", err)
		}

		fmt.Printf("Stop reason: %s\n", resp.StopReason)
		fmt.Printf("Tokens: %d in / %d out (total: %d)\n", resp.InputTokens, resp.OutputTokens, resp.TotalTokens())

		if resp.Text != "" {
			fmt.Printf("Text: %s\n", resp.Text)
		}

		// Add assistant response to memory
		if err := mem.AddAssistantResponse(resp); err != nil {
			log.Fatalf("AddAssistantResponse error: %v", err)
		}

		if resp.StopReason == llm.StopReasonEndTurn {
			fmt.Println("\nDone.")
			break
		}

		if resp.StopReason == llm.StopReasonToolUse {
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
			if err := mem.AddToolResult(results); err != nil {
				log.Fatalf("AddToolResult error: %v", err)
			}
		}
	}

	// Show final conversation stats
	n, _ := mem.Len()
	fmt.Printf("\nConversation: %d messages in memory\n", n)
}
