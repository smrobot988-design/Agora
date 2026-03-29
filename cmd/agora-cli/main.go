package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/smrobot988-design/Agora/pkg/config"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/tool"
	"github.com/smrobot988-design/Agora/pkg/tool/builtin"
)

var (
	flagModel    = flag.String("model", "", "Claude model to use (default: claude-sonnet-4-6)")
	flagMaxTurns = flag.Int("max-turns", 10, "Maximum agent loop turns per input")
	flagSystem   = flag.String("system", "You are a helpful assistant with access to tools for reading files and running shell commands.", "System prompt")
	flagTimeout  = flag.Int("timeout", 30, "Default command timeout in seconds")
	flagMaxOut   = flag.Int("max-output", 102400, "Maximum command output size in bytes")
)

func main() {
	flag.Parse()

	// if os.Getenv("ANTHROPIC_API_KEY") == "" {
	// 	fmt.Fprintln(os.Stderr, "Error: ANTHROPIC_API_KEY environment variable is required")
	// 	os.Exit(1)
	// }

	// Build provider options
	var providerOpts []llm.ClaudeOption
	if *flagModel != "" {
		providerOpts = append(providerOpts, llm.WithModel(anthropic.Model(*flagModel)))
	}
	if baseURL := os.Getenv("ANTHROPIC_BASE_URL"); baseURL != "" {
		providerOpts = append(providerOpts, llm.WithBaseURL(baseURL))
	}

	provider := llm.NewClaudeProvider(providerOpts...)

	// Register built-in tools
	registry := tool.NewRegistry()
	mustRegister(registry, builtin.NewReadFile())
	mustRegister(registry, builtin.NewRunCommand(
		builtin.WithDefaultTimeout(*flagTimeout),
		builtin.WithMaxOutput(*flagMaxOut),
	))

	// Create memory (shared across all turns for multi-turn conversation)
	mem := core.NewMemory(core.WithSystemPrompt(*flagSystem))

	// Create agent
	agent := core.NewAgent(provider, mem, registry, core.WithMaxTurns(*flagMaxTurns))

	// Set up signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	runREPL(ctx, agent)
}

func mustRegister(registry *tool.Registry, t tool.Tool) {
	if err := registry.Register(t); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register tool %q: %v\n", t.Definition().Name, err)
		os.Exit(1)
	}
}

func runREPL(ctx context.Context, agent *core.Agent) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Agora Agent (type 'exit' or 'quit' to stop)")
	fmt.Println()

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break // EOF or error
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		result, err := agent.Run(ctx, input)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println("\nInterrupted.")
				return
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		fmt.Println()
		fmt.Println(result.Text)
		fmt.Printf("\n[tokens: %d in / %d out | turns: %d]\n\n",
			result.TotalInputTokens, result.TotalOutputTokens, result.Turns)
	}

	if err := scanner.Err(); err != nil {
		slog.Warn("input error", "err", err)
	}
}
