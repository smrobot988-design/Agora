package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
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
	Name() string
}

var (
	flagProvider        = flag.String("provider", "minimax", "Provider: claude, minimax, deepseek, doubao, kimi, glm, gpt")
	flagAPIKey          = flag.String("api-key", "", "API key (or set MINIMAX_API_KEY / ANTHROPIC_API_KEY env var)")
	flagBaseURL         = flag.String("base-url", "", "Custom API base URL (e.g. for proxy/relay)")
	flagModel           = flag.String("model", "", "Override model name (currently claude only; can also use ANTHROPIC_MODEL)")
	flagReasoningMode   = flag.String("reasoning-mode", "auto", "Reasoning parsing mode: auto, none, think-tag, native")
	flagThinkingMode    = flag.String("thinking-mode", "", "Provider thinking mode override: on, off, auto (empty keeps provider default)")
	flagThinkingEffort  = flag.String("thinking-effort", "", "Provider thinking effort override: low, medium, high, max")
	flagThinkingBudget  = flag.Int("thinking-budget", 0, "Provider thinking budget tokens (if supported)")
	flagShowReasoning   = flag.Bool("show-reasoning", false, "Show reasoning output in real time")
	flagReasoningOutput = flag.String("reasoning-output", "stderr", "Where to write reasoning output: stderr, stdout, hidden")
	flagDebugEvents     = flag.Bool("debug-events", false, "Print streaming event debug info to stderr")
	flagTask            = flag.String("task", "", "Run a single task and exit (non-REPL mode). Example: -task='你好，帮我看一下当前项目有哪些文件。'")
)

func main() {
	flag.Parse()

	display := streamDisplayOptions{
		showReasoning:   *flagShowReasoning,
		reasoningOutput: *flagReasoningOutput,
		debugEvents:     *flagDebugEvents,
	}
	reasoningConfig, err := buildReasoningConfig(*flagReasoningMode, *flagThinkingMode, *flagThinkingEffort, *flagThinkingBudget)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	provider := newStreamProvider(*flagProvider, *flagAPIKey, *flagBaseURL, *flagModel, *flagReasoningMode)

	// Register tools
	registry := tool.NewRegistry()
	mustRegister(registry, builtin.NewReadFile())
	mustRegister(registry, builtin.NewRunCommand())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if *flagTask != "" {
		runStreamTask(ctx, provider, registry, *flagTask, display, reasoningConfig)
	} else {
		runStreamREPL(ctx, provider, registry, display, reasoningConfig)
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

type streamDisplayOptions struct {
	showReasoning   bool
	reasoningOutput string
	debugEvents     bool
}

type streamedTurn struct {
	textDelta        string
	reasoningDelta   string
	appliedReasoning *llm.AppliedReasoning
	stopReason       llm.StopReason
	toolCalls        []*activeToolCall
	hadToolDelta     bool
	inputTokens      int
	outputTokens     int
}

func streamChatTurn(ctx context.Context, provider StreamProvider, params llm.ChatParams, display streamDisplayOptions) (*streamedTurn, error) {
	result := &streamedTurn{}
	var currentTool *activeToolCall

	showReasoning := display.showReasoning && strings.ToLower(display.reasoningOutput) != "hidden"
	reasoningWriter := reasoningOutputWriter(display.reasoningOutput)
	reasoningSectionOpen := false
	closeReasoningSection := func() {
		if showReasoning && reasoningSectionOpen {
			fmt.Fprintln(reasoningWriter)
			reasoningSectionOpen = false
		}
	}

	err := provider.ChatStream(ctx, params, func(pr *llm.PartialResponse) {
		if pr == nil {
			closeReasoningSection()
			return
		}
		if display.debugEvents {
			debugStreamEvent(pr)
		}

		switch pr.Type {
		case llm.StreamEventTextDelta:
			closeReasoningSection()
			fmt.Print(pr.TextDelta)
			result.textDelta += pr.TextDelta

		case llm.StreamEventReasoningDelta:
			result.reasoningDelta += pr.ReasoningDelta
			if showReasoning {
				if !reasoningSectionOpen {
					fmt.Fprint(reasoningWriter, "\n[reasoning] ")
					reasoningSectionOpen = true
				}
				fmt.Fprint(reasoningWriter, pr.ReasoningDelta)
			}

		case llm.StreamEventReasoningApplied:
			result.appliedReasoning = pr.AppliedReasoning

		case llm.StreamEventToolDelta:
			closeReasoningSection()
			result.hadToolDelta = true
			tc := pr.ToolCallDelta
			if tc == nil {
				return
			}
			if currentTool == nil || currentTool.index != tc.Index {
				currentTool = &activeToolCall{index: tc.Index}
				result.toolCalls = append(result.toolCalls, currentTool)
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

		case llm.StreamEventUsage:
			closeReasoningSection()
			result.inputTokens = pr.InputTokens
			result.outputTokens = pr.OutputTokens

		case llm.StreamEventStop:
			closeReasoningSection()
			result.stopReason = pr.StopReason
		}
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func reasoningOutputWriter(target string) io.Writer {
	switch strings.ToLower(target) {
	case "stdout":
		return os.Stdout
	case "hidden":
		return io.Discard
	default:
		return os.Stderr
	}
}

func debugStreamEvent(pr *llm.PartialResponse) {
	switch pr.Type {
	case llm.StreamEventTextDelta:
		fmt.Fprintf(os.Stderr, "[event] type=%s len=%d\n", pr.Type, len(pr.TextDelta))
	case llm.StreamEventReasoningDelta:
		fmt.Fprintf(os.Stderr, "[event] type=%s len=%d\n", pr.Type, len(pr.ReasoningDelta))
	case llm.StreamEventReasoningApplied:
		if pr.AppliedReasoning == nil {
			fmt.Fprintf(os.Stderr, "[event] type=%s\n", pr.Type)
			return
		}
		fmt.Fprintf(os.Stderr, "[event] type=%s provider=%s source=%s model=%q mode=%s effort=%s budget=%d parse_mode=%s notes=%d\n",
			pr.Type,
			pr.AppliedReasoning.Provider,
			pr.AppliedReasoning.Source,
			pr.AppliedReasoning.Model,
			pr.AppliedReasoning.Mode,
			pr.AppliedReasoning.Effort,
			pr.AppliedReasoning.BudgetTokens,
			pr.AppliedReasoning.ParseMode,
			len(pr.AppliedReasoning.Notes),
		)
	case llm.StreamEventToolDelta:
		if pr.ToolCallDelta == nil {
			fmt.Fprintf(os.Stderr, "[event] type=%s\n", pr.Type)
			return
		}
		fmt.Fprintf(os.Stderr, "[event] type=%s index=%d id=%q name=%q args_len=%d\n",
			pr.Type, pr.ToolCallDelta.Index, pr.ToolCallDelta.ID, pr.ToolCallDelta.Name, len(pr.ToolCallDelta.ArgumentsDelta))
	case llm.StreamEventUsage:
		fmt.Fprintf(os.Stderr, "[event] type=%s input_tokens=%d output_tokens=%d\n", pr.Type, pr.InputTokens, pr.OutputTokens)
	case llm.StreamEventStop:
		fmt.Fprintf(os.Stderr, "[event] type=%s stop_reason=%s\n", pr.Type, pr.StopReason)
	default:
		fmt.Fprintf(os.Stderr, "[event] type=%s\n", pr.Type)
	}
}

func printTurnSummary(provider string, turn int, result *streamedTurn) {
	fmt.Fprintf(os.Stderr,
		"\n[SUMMARY] provider=%s turn=%d stop_reason=%s text_len=%d reasoning_len=%d tool_calls=%d input_tokens=%d output_tokens=%d\n",
		provider, turn, result.stopReason, len(result.textDelta), len(result.reasoningDelta), len(result.toolCalls), result.inputTokens, result.outputTokens,
	)
	if result.appliedReasoning != nil {
		fmt.Fprintf(os.Stderr,
			"[SUMMARY] reasoning provider=%s source=%s model=%q mode=%s effort=%s budget=%d parse_mode=%s notes=%v\n",
			result.appliedReasoning.Provider,
			result.appliedReasoning.Source,
			result.appliedReasoning.Model,
			result.appliedReasoning.Mode,
			result.appliedReasoning.Effort,
			result.appliedReasoning.BudgetTokens,
			result.appliedReasoning.ParseMode,
			result.appliedReasoning.Notes,
		)
	}
}

// runStreamTask runs a single task with streaming output and tool execution, then exits.
func runStreamTask(ctx context.Context, provider StreamProvider, registry *tool.Registry, task string, display streamDisplayOptions, reasoningConfig llm.ReasoningConfig) {
	var history []llm.Message
	history = append(history, llm.NewTextMessage(llm.RoleUser, task))

	fmt.Printf("Task: %s\n\n", task)

	for turn := 0; turn < 20; turn++ {
		result, err := streamChatTurn(ctx, provider, llm.ChatParams{
			Messages:  history,
			Tools:     registry.Definitions(),
			Reasoning: &reasoningConfig,
		}, display)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			os.Exit(1)
		}

		fmt.Println()
		printTurnSummary(provider.Name(), turn+1, result)

		if result.hadToolDelta || result.stopReason == llm.StopReasonToolUse {
			var blocks []llm.ContentBlock
			for _, tc := range result.toolCalls {
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
		} else if result.textDelta != "" {
			history = append(history, llm.NewTextMessage(llm.RoleAssistant, result.textDelta))
		}

		if len(result.toolCalls) == 0 {
			break
		}

		for _, tc := range result.toolCalls {
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
func runStreamREPL(ctx context.Context, provider StreamProvider, registry *tool.Registry, display streamDisplayOptions, reasoningConfig llm.ReasoningConfig) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Agora Streaming REPL")
	fmt.Println("Usage: go run ./cmd/stream/ -provider claude|minimax|deepseek|doubao|kimi|glm|gpt [-api-key KEY] [-base-url URL] [-model NAME] [-thinking-mode on|off|auto] [-thinking-effort low|medium|high|max] [-thinking-budget TOKENS] [-reasoning-mode auto|none|think-tag|native] [-show-reasoning]")
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
			fmt.Println()

			result, err := streamChatTurn(ctx, provider, llm.ChatParams{
				Messages:  history,
				Tools:     tools,
				Reasoning: &reasoningConfig,
			}, display)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
				history = history[:len(history)-1]
				break
			}

			fmt.Println()
			printTurnSummary(provider.Name(), turn+1, result)

			// Append assistant message to history based on stop reason.
			if result.hadToolDelta || result.stopReason == llm.StopReasonToolUse {
				// Assistant message must contain tool_use blocks for MiniMax to correlate tool results.
				var blocks []llm.ContentBlock
				for _, tc := range result.toolCalls {
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
			} else if result.textDelta != "" {
				history = append(history, llm.NewTextMessage(llm.RoleAssistant, result.textDelta))
			}

			// If no tool calls, we're done with this turn.
			if len(result.toolCalls) == 0 || result.stopReason != llm.StopReasonToolUse {
				break
			}

			// Execute each tool call and append results to history.
			for _, tc := range result.toolCalls {
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
func newStreamProvider(providerFlag, apiKey, baseURL, modelName, reasoningModeFlag string) StreamProvider {
	reasoningMode, reasoningModeSet, err := parseReasoningMode(reasoningModeFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

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
		if reasoningModeSet {
			opts = append(opts, llm.MiniMaxWithReasoningMode(reasoningMode))
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
		if model := firstNonEmpty(modelName, os.Getenv("ANTHROPIC_MODEL")); model != "" {
			opts = append(opts, llm.WithModelName(model))
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
		if reasoningModeSet {
			opts = append(opts, llm.DeepseekWithReasoningMode(reasoningMode))
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
		if reasoningModeSet {
			opts = append(opts, llm.DoubaoWithReasoningMode(reasoningMode))
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
		if reasoningModeSet {
			opts = append(opts, llm.KimiWithReasoningMode(reasoningMode))
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
		if reasoningModeSet {
			opts = append(opts, llm.GLMWithReasoningMode(reasoningMode))
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
		if reasoningModeSet {
			opts = append(opts, llm.GPTWithReasoningMode(reasoningMode))
		}
		return llm.NewGPTProvider(opts...)

	default:
		fmt.Fprintf(os.Stderr, "unknown provider %q (use claude, minimax, deepseek, doubao, kimi, glm, gpt)\n", providerFlag)
		os.Exit(1)
		return nil
	}
}

func parseReasoningMode(value string) (llm.ReasoningMode, bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "", false, nil
	case "none":
		return llm.ReasoningModeNone, true, nil
	case "think", "think-tag", "think_tag":
		return llm.ReasoningModeThinkTag, true, nil
	case "native":
		return llm.ReasoningModeNative, true, nil
	default:
		return "", false, fmt.Errorf("unknown reasoning mode %q (use auto, none, think-tag, native)", value)
	}
}

func buildReasoningConfig(parseMode, thinkingMode, thinkingEffort string, thinkingBudget int) (llm.ReasoningConfig, error) {
	config := llm.ReasoningConfig{}

	switch strings.ToLower(strings.TrimSpace(thinkingMode)) {
	case "", "inherit":
	case "on":
		config.Mode = llm.ThinkingModeOn
	case "off":
		config.Mode = llm.ThinkingModeOff
	case "auto":
		config.Mode = llm.ThinkingModeAuto
	default:
		return llm.ReasoningConfig{}, fmt.Errorf("unknown thinking mode %q (use on, off, auto)", thinkingMode)
	}

	switch strings.ToLower(strings.TrimSpace(thinkingEffort)) {
	case "", "inherit":
	case "low":
		config.Effort = llm.ThinkingEffortLow
	case "medium":
		config.Effort = llm.ThinkingEffortMedium
	case "high":
		config.Effort = llm.ThinkingEffortHigh
	case "max":
		config.Effort = llm.ThinkingEffortMax
	default:
		return llm.ReasoningConfig{}, fmt.Errorf("unknown thinking effort %q (use low, medium, high, max)", thinkingEffort)
	}

	parse, parseSet, err := parseReasoningMode(parseMode)
	if err != nil {
		return llm.ReasoningConfig{}, err
	}
	if parseSet {
		config.ParseMode = parse
	}
	if thinkingBudget < 0 {
		return llm.ReasoningConfig{}, fmt.Errorf("thinking-budget must be >= 0")
	}
	config.BudgetTokens = thinkingBudget

	return config.Normalize(), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
