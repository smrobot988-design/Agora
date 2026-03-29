package main

import (
	"context"
	"fmt"
	"log"
	"os"

	_ "github.com/smrobot988-design/Agora/pkg/config"
	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

func main() {
	// if os.Getenv("ANTHROPIC_API_KEY") == "" {
	// 	log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	// }

	opts := []llm.ClaudeOption{}
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		opts = append(opts, llm.WithAPIKey(apiKey))
	}
	if baseURL := os.Getenv("ANTHROPIC_BASE_URL"); baseURL != "" {
		opts = append(opts, llm.WithBaseURL(baseURL))
	}
	if len(opts) == 0 {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	}

	provider := llm.NewClaudeProvider(opts...)
	fmt.Printf("Provider: %s\n", provider.Name())

	// Register tools
	registry := tool.NewRegistry()
	if err := registry.Register(&tool.Func{
		Def: schema.ToolDefinition{
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
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			city, _ := input["city"].(string)
			return fmt.Sprintf(`{"city": %q, "temperature": "18°C", "condition": "partly cloudy", "humidity": "72%%"}`, city), nil
		},
	}); err != nil {
		log.Fatalf("Register tool error: %v", err)
	}

	// Create Memory with system prompt
	mem := core.NewMemory(
		core.WithSystemPrompt("You are a helpful weather assistant. Answer concisely."),
	)

	// Create Agent and run
	agent := core.NewAgent(provider, mem, registry, core.WithMaxTurns(5))

	ctx := context.Background()
	result, err := agent.Run(ctx, "What's the weather in San Francisco?")
	if err != nil {
		log.Fatalf("Agent run error: %v", err)
	}

	fmt.Printf("\nResponse: %s\n", result.Text)
	fmt.Printf("Tokens: %d in / %d out (total: %d)\n",
		result.TotalInputTokens, result.TotalOutputTokens, result.TotalTokens())
	fmt.Printf("Turns: %d\n", result.Turns)

	n, _ := mem.Len()
	fmt.Printf("Conversation: %d messages in memory\n", n)
}
