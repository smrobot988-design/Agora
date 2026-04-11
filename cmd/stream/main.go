package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	_ "github.com/smrobot988-design/Agora/pkg/config"
	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
	"github.com/smrobot988-design/Agora/pkg/tool/builtin"
)

// StreamProvider abstracts both Claude and MiniMax streaming providers.
type StreamProvider interface {
	ChatStream(ctx context.Context, params llm.ChatParams, cb func(*llm.PartialResponse)) error
}

// thinkRegex strips <think>...</think> blocks (MiniMax-M2 emits these as text_delta).
var thinkRegex = regexp.MustCompile(`(?s)<think>.*?</think>\s*`)

var (
	flagProvider = flag.String("provider", "minimax", "Provider: claude, minimax, deepseek, doubao, kimi, glm, gpt")
	flagAPIKey   = flag.String("api-key", "", "API key (or set MINIMAX_API_KEY / ANTHROPIC_API_KEY env var)")
	flagBaseURL  = flag.String("base-url", "", "Custom API base URL (e.g. for proxy/relay)")
	flagTask     = flag.String("task", "", "Run a single task and exit (non-REPL mode). Example: -task='你好，帮我看一下当前项目有哪些文件。'")
)

func main() {
	flag.Parse()

	provider := newStreamProvider(*flagProvider, *flagAPIKey, *flagBaseURL)

	// Register tools
	registry := tool.NewRegistry()
	mustRegister(registry, builtin.NewReadFile())
	mustRegister(registry, builtin.NewRunCommand())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if *flagTask != "" {
		runStreamTask(ctx, provider, registry, *flagTask)
	} else {
		runStreamREPL(ctx, provider, registry)
	}
}

func mustRegister(registry *tool.Registry, t tool.Tool) {
	if err := registry.Register(t); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register tool %q: %v\n", t.Definition().Name, err)
		os.Exit(1)
	}
}

// activeToolCall accumulates streaming tool call deltas.
type activeToolCall struct {
	index     int
	id        string
	name      string
	arguments string
}

// runStreamTask runs a single task with streaming output and tool execution, then exits.
func runStreamTask(ctx context.Context, provider StreamProvider, registry *tool.Registry, task string) {
	var history []llm.Message
	history = append(history, llm.NewTextMessage(llm.RoleUser, task))

	fmt.Printf("Task: %s\n\n", task)

	for turn := 0; turn < 20; turn++ {
		var textDelta string
		var stopReason llm.StopReason
		var toolCalls []*activeToolCall
		var currentTool *activeToolCall
		var hadToolDelta bool

		err := provider.ChatStream(ctx, llm.ChatParams{
			Messages: history,
			Tools:    registry.Definitions(),
		}, func(pr *llm.PartialResponse) {
			if pr == nil {
				return
			}
			switch pr.Type {
			case llm.StreamEventTextDelta:
				fmt.Print(pr.TextDelta)
				textDelta += pr.TextDelta
			case llm.StreamEventToolDelta:
				hadToolDelta = true
				tc := pr.ToolCallDelta
				if tc == nil {
					return
				}
				if currentTool == nil || currentTool.index != tc.Index {
					currentTool = &activeToolCall{index: tc.Index}
					toolCalls = append(toolCalls, currentTool)
				}
				if tc.ID != "" {
					currentTool.id = tc.ID
				}
				if tc.Name != "" {
					currentTool.name = tc.Name
				}
				if tc.ArgumentsDelta != "" {
					currentTool.arguments += tc.ArgumentsDelta
				}
			case llm.StreamEventStop:
				stopReason = pr.StopReason
			}
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			os.Exit(1)
		}

		fmt.Println()

		// Append assistant message to history.
		if hadToolDelta || stopReason == llm.StopReasonToolUse {
			var blocks []llm.ContentBlock
			for _, tc := range toolCalls {
				if tc.name == "" {
					continue
				}
				blocks = append(blocks, llm.ContentBlock{
					Type: llm.BlockToolUse,
					ToolCall: schema.ToolCall{
						ID:    tc.id,
						Name:  tc.name,
						Input: parseJSON(tc.arguments),
					},
				})
			}
			if len(blocks) > 0 {
				history = append(history, llm.Message{Role: llm.RoleAssistant, Content: blocks})
			}
		} else {
			cleanText := thinkRegex.ReplaceAllString(textDelta, "")
			if cleanText != "" {
				history = append(history, llm.NewTextMessage(llm.RoleAssistant, cleanText))
			}
		}

		// No tool calls -> done.
		fmt.Fprintf(os.Stderr, "\n[DEBUG] turn %d stop_reason=%s tool_calls=%d\n", turn, stopReason, len(toolCalls))
		if len(toolCalls) == 0 {
			break
		}

		// Execute tools.
		for _, tc := range toolCalls {
			if tc.name == "" {
				continue
			}
			t, ok := registry.Get(tc.name)
			if !ok {
				history = append(history, llm.NewToolResultMessage([]llm.ToolResult{{
					ToolCallID: tc.id,
					Content:    fmt.Sprintf("error: tool %q not found", tc.name),
					IsError:    true,
				}}))
				continue
			}
			fmt.Fprintf(os.Stderr, "\n[tool: %s args: %v]\n", tc.name, parseJSON(tc.arguments))
			output, err := t.Execute(ctx, parseJSON(tc.arguments))
			if err != nil {
				output = fmt.Sprintf("error: %v", err)
			}
			history = append(history, llm.NewToolResultMessage([]llm.ToolResult{{
				ToolCallID: tc.id,
				Content:    output,
				IsError:    err != nil,
			}}))
		}
	}
}

// runStreamREPL is the interactive REPL mode.
func runStreamREPL(ctx context.Context, provider StreamProvider, registry *tool.Registry) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Agora Streaming REPL")
	fmt.Println("Usage: go run ./cmd/stream/ -provider claude|minimax|deepseek|doubao|kimi|glm|gpt [-api-key KEY] [-base-url URL]")
	fmt.Println("Tools: read_file, run_command")
	fmt.Println("Tools: read_file, run_command")
	fmt.Println("Type 'exit' or 'quit' to stop, 'clear' to clear history")
	fmt.Println()

	var history []llm.Message

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}
		if input == "clear" {
			history = nil
			fmt.Println("(history cleared)")
			continue
		}

		history = append(history, llm.NewTextMessage(llm.RoleUser, input))
		tools := registry.Definitions()

		for turn := 0; turn < 20; turn++ {
			var textDelta string
			var stopReason llm.StopReason
			var toolCalls []*activeToolCall
			var currentTool *activeToolCall
			var hadToolDelta bool

			fmt.Println()

			err := provider.ChatStream(ctx, llm.ChatParams{
				Messages: history,
				Tools:    tools,
			}, func(pr *llm.PartialResponse) {
				if pr == nil {
					return
				}
				switch pr.Type {
				case llm.StreamEventTextDelta:
					fmt.Print(pr.TextDelta)
					textDelta += pr.TextDelta

				case llm.StreamEventToolDelta:
					hadToolDelta = true
					tc := pr.ToolCallDelta
					if tc == nil {
						return
					}
					if currentTool == nil || currentTool.index != tc.Index {
						currentTool = &activeToolCall{index: tc.Index}
						toolCalls = append(toolCalls, currentTool)
					}
					if tc.ID != "" {
						currentTool.id = tc.ID
					}
					if tc.Name != "" {
						currentTool.name = tc.Name
					}
					if tc.ArgumentsDelta != "" {
						currentTool.arguments += tc.ArgumentsDelta
					}

				case llm.StreamEventStop:
					stopReason = pr.StopReason
				}
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
				history = history[:len(history)-1]
				break
			}

			fmt.Println()

			// Append assistant message to history based on stop reason.
			if hadToolDelta || stopReason == llm.StopReasonToolUse {
				// Assistant message must contain tool_use blocks for MiniMax to correlate tool results.
				var blocks []llm.ContentBlock
				for _, tc := range toolCalls {
					if tc.name == "" {
						continue
					}
					blocks = append(blocks, llm.ContentBlock{
						Type: llm.BlockToolUse,
						ToolCall: schema.ToolCall{
							ID:    tc.id,
							Name:  tc.name,
							Input: parseJSON(tc.arguments),
						},
					})
				}
				if len(blocks) > 0 {
					history = append(history, llm.Message{Role: llm.RoleAssistant, Content: blocks})
				}
			} else {
				// Final text response: strip think tags and store as text.
				cleanText := thinkRegex.ReplaceAllString(textDelta, "")
				if cleanText != "" {
					history = append(history, llm.NewTextMessage(llm.RoleAssistant, cleanText))
				}
			}

			// If no tool calls, we're done with this turn.
			if len(toolCalls) == 0 || stopReason != llm.StopReasonToolUse {
				break
			}

			// Execute each tool call and append results to history.
			for _, tc := range toolCalls {
				if tc.name == "" {
					continue
				}

				t, ok := registry.Get(tc.name)
				if !ok {
					history = append(history, llm.NewToolResultMessage([]llm.ToolResult{{
						ToolCallID: tc.id,
						Content:    fmt.Sprintf("error: tool %q not found", tc.name),
						IsError:    true,
					}}))
					continue
				}

				fmt.Fprintf(os.Stderr, "\n[tool: %s args: %v]\n", tc.name, parseJSON(tc.arguments))
				output, err := t.Execute(ctx, parseJSON(tc.arguments))
				if err != nil {
					output = fmt.Sprintf("error: %v", err)
				}
				history = append(history, llm.NewToolResultMessage([]llm.ToolResult{{
					ToolCallID: tc.id,
					Content:    output,
					IsError:    err != nil,
				}}))
			}
		}
		fmt.Println()
	}
}

// parseJSON attempts to parse s as JSON. Returns s as a map on success, or nil on failure.
func parseJSON(s string) map[string]interface{} {
	if s == "" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

// newStreamProvider creates a StreamProvider based on command-line flags and env vars.
func newStreamProvider(providerFlag, apiKey, baseURL string) StreamProvider {
	switch strings.ToLower(providerFlag) {
	case "minimax":
		opts := []llm.MiniMaxOption{}
		key := firstNonEmpty(apiKey, os.Getenv("MINIMAX_API_KEY"))
		if key == "" {
			fmt.Fprintln(os.Stderr, "Error: MINIMAX_API_KEY not set (use -api-key or set env)")
			os.Exit(1)
		}
		opts = append(opts, llm.MiniMaxWithAPIKey(key))
		if baseURL != "" {
			opts = append(opts, llm.MiniMaxWithBaseURL(baseURL))
		}
		return llm.NewMiniMaxProvider(opts...)

	case "claude":
		opts := []llm.ClaudeOption{}
		key := firstNonEmpty(apiKey, os.Getenv("ANTHROPIC_API_KEY"))
		if key == "" {
			fmt.Fprintln(os.Stderr, "Error: ANTHROPIC_API_KEY not set (use -api-key or set env)")
			os.Exit(1)
		}
		opts = append(opts, llm.WithAPIKey(key))
		if url := firstNonEmpty(baseURL, os.Getenv("ANTHROPIC_BASE_URL")); url != "" {
			opts = append(opts, llm.WithBaseURL(url))
		}
		return llm.NewClaudeProvider(opts...)

	case "deepseek":
		opts := []llm.DeepseekOption{}
		key := firstNonEmpty(apiKey, os.Getenv("DEEPSEEK_API_KEY"))
		if key == "" {
			fmt.Fprintln(os.Stderr, "Error: DEEPSEEK_API_KEY not set (use -api-key or set env)")
			os.Exit(1)
		}
		opts = append(opts, llm.DeepseekWithAPIKey(key))
		if baseURL != "" {
			opts = append(opts, llm.DeepseekWithBaseURL(baseURL))
		}
		return llm.NewDeepseekProvider(opts...)

	case "doubao":
		opts := []llm.DoubaoOption{}
		key := firstNonEmpty(apiKey, os.Getenv("ARK_API_KEY"))
		if key == "" {
			fmt.Fprintln(os.Stderr, "Error: ARK_API_KEY not set (use -api-key or set env)")
			os.Exit(1)
		}
		opts = append(opts, llm.DoubaoWithAPIKey(key))
		if model := os.Getenv("ARK_MODEL"); model != "" {
			opts = append(opts, llm.DoubaoWithModel(model))
		}
		if baseURL != "" {
			opts = append(opts, llm.DoubaoWithBaseURL(baseURL))
		}
		return llm.NewDoubaoProvider(opts...)

	case "kimi":
		opts := []llm.KimiOption{}
		key := firstNonEmpty(apiKey, os.Getenv("MOONSHOT_API_KEY"))
		if key == "" {
			fmt.Fprintln(os.Stderr, "Error: MOONSHOT_API_KEY not set (use -api-key or set env)")
			os.Exit(1)
		}
		opts = append(opts, llm.KimiWithAPIKey(key))
		if baseURL != "" {
			opts = append(opts, llm.KimiWithBaseURL(baseURL))
		}
		return llm.NewKimiProvider(opts...)

	case "glm":
		opts := []llm.GLMOption{}
		key := firstNonEmpty(apiKey, os.Getenv("GLM_API_KEY"))
		if key == "" {
			fmt.Fprintln(os.Stderr, "Error: GLM_API_KEY not set (use -api-key or set env)")
			os.Exit(1)
		}
		opts = append(opts, llm.GLMWithAPIKey(key))
		if baseURL != "" {
			opts = append(opts, llm.GLMWithBaseURL(baseURL))
		}
		return llm.NewGLMProvider(opts...)

	case "gpt":
		opts := []llm.GPTOption{}
		key := firstNonEmpty(apiKey, os.Getenv("OPENAI_API_KEY"))
		if key == "" {
			fmt.Fprintln(os.Stderr, "Error: OPENAI_API_KEY not set (use -api-key or set env)")
			os.Exit(1)
		}
		opts = append(opts, llm.GPTWithAPIKey(key))
		if baseURL != "" {
			opts = append(opts, llm.GPTWithBaseURL(baseURL))
		}
		return llm.NewGPTProvider(opts...)

	default:
		fmt.Fprintf(os.Stderr, "unknown provider %q (use claude, minimax, deepseek, doubao, kimi, glm, gpt)\n", providerFlag)
		os.Exit(1)
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
