package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

// handoffPrefix is the structured signal prefix that HandoffTool returns.
const (
	handoffPrefix          = "HANDOFF:"
	defaultMaxHandoffCount = 10
)

// SwarmRunner implements the Handoff/Swarm pattern.
// Agents can transfer control to other agents by calling a HandoffTool.
// Unlike Supervisor (where control returns to the coordinator), in Swarm
// the original agent exits and the target agent fully takes over.
//
// Handoff detection reads tool results directly from the Runner
// (via RunnerWithToolResults), which is more reliable than parsing LLM text.
type SwarmRunner struct {
	name        string
	runners     map[string]Runner
	entryPoint  string
	maxHandoffs int
}

// SwarmOption configures a SwarmRunner.
type SwarmOption func(*SwarmRunner)

// WithMaxHandoffs sets the maximum number of handoffs allowed.
func WithMaxHandoffs(n int) SwarmOption {
	return func(s *SwarmRunner) { s.maxHandoffs = n }
}

// NewSwarmRunner creates a swarm with named runners.
func NewSwarmRunner(name, entryPoint string, runners map[string]Runner, opts ...SwarmOption) *SwarmRunner {
	s := &SwarmRunner{
		name:        name,
		runners:     runners,
		entryPoint:  entryPoint,
		maxHandoffs: defaultMaxHandoffCount,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run executes the swarm starting from the entry point.
// Handoff signals are detected by reading tool results from the runner,
// not by parsing LLM text output.
func (s *SwarmRunner) Run(ctx context.Context, input string) (*core.Result, error) {
	current := s.entryPoint
	currentInput := input

	aggregated := &core.Result{}

	for handoff := 0; handoff <= s.maxHandoffs; handoff++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("swarm %s: context cancelled: %w", s.name, err)
		}

		runner, ok := s.runners[current]
		if !ok {
			return nil, fmt.Errorf("swarm %s: unknown agent %q", s.name, current)
		}

		slog.Info("swarm executing agent",
			"swarm", s.name,
			"agent", current,
			"handoff", handoff,
		)

		result, err := runner.Run(ctx, currentInput)
		if err != nil {
			return nil, fmt.Errorf("swarm %s agent %q: %w", s.name, current, err)
		}

		// Aggregate token usage.
		aggregated.TotalInputTokens += result.TotalInputTokens
		aggregated.TotalOutputTokens += result.TotalOutputTokens
		aggregated.Turns += result.Turns

		// Try to detect handoff from tool results (via RunnerWithToolResults).
		target, message := s.detectHandoff(runner)
		if target != "" {
			// Handoff detected from tool results — LLM text is ignored.
			aggregated.Text = result.Text
			slog.Info("swarm handoff",
				"from", current,
				"to", target,
				"via", "tool_result",
			)
			current = target
			currentInput = message
			continue
		}

		// No handoff in tool results — terminal result.
		aggregated.Text = result.Text
		slog.Info("swarm completed",
			"swarm", s.name,
			"final_agent", current,
			"total_handoffs", handoff,
		)
		return aggregated, nil
	}

	return nil, fmt.Errorf("swarm %s: max handoffs (%d) exceeded", s.name, s.maxHandoffs)
}

// detectHandoff checks if the runner's last tool results contain a handoff signal.
// Returns (target, context) if found, ("", "") if not.
func (s *SwarmRunner) detectHandoff(runner Runner) (target, context string) {
	rwt, ok := runner.(RunnerWithToolResults)
	if !ok {
		return "", ""
	}

	results, err := rwt.LastToolResults()
	if err != nil || len(results) == 0 {
		return "", ""
	}

	// Check each tool result for the handoff prefix.
	for _, tr := range results {
		if strings.HasPrefix(tr.Content, handoffPrefix) {
			return parseHandoffFromContent(tr.Content)
		}
		// Also try parsing as JSON.
		if t, c := parseHandoffFromJSON(tr.Content); t != "" {
			return t, c
		}
	}
	return "", ""
}

// Name returns the swarm's name.
func (s *SwarmRunner) Name() string { return s.name }

// parseHandoffFromContent extracts target and context from a HANDOFF: prefixed string.
func parseHandoffFromContent(content string) (target, context string) {
	rest := content[len(handoffPrefix):]
	idx := strings.Index(rest, ":")
	if idx < 0 {
		return rest, ""
	}
	return rest[:idx], rest[idx+1:]
}

// handoffJSON is the JSON handoff signal format.
type handoffJSON struct {
	Target  string `json:"target"`
	Context string `json:"context"`
}

// parseHandoffFromJSON extracts target and context from a JSON handoff signal.
func parseHandoffFromJSON(content string) (target, context string) {
	var h handoffJSON
	if err := json.Unmarshal([]byte(content), &h); err == nil && h.Target != "" {
		return h.Target, h.Context
	}
	return "", ""
}

// HandoffTool is a tool that agents in a swarm call to signal a handoff.
type HandoffTool struct {
	availableAgents []string
}

// Compile-time interface check.
var _ tool.Tool = (*HandoffTool)(nil)

// NewHandoffTool creates a handoff tool listing the available target agents.
func NewHandoffTool(agentNames []string) *HandoffTool {
	return &HandoffTool{availableAgents: agentNames}
}

// Definition returns the tool schema for the LLM.
func (t *HandoffTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        "handoff",
		Description: fmt.Sprintf("Transfer control to another agent. Available agents: %v. Use this when the task requires a different specialist.", t.availableAgents),
		InputSchema: schema.PropertySchema{
			Properties: map[string]interface{}{
				"target_agent": map[string]interface{}{
					"type":        "string",
					"description": "Name of the agent to hand off to",
					"enum":        t.availableAgents,
				},
				"context": map[string]interface{}{
					"type":        "string",
					"description": "Context and instructions to pass to the target agent",
				},
			},
			Required: []string{"target_agent", "context"},
		},
	}
}

// Execute returns a structured handoff signal that SwarmRunner parses
// from tool results (supports both HANDOFF: prefix and JSON formats).
func (t *HandoffTool) Execute(_ context.Context, input map[string]interface{}) (string, error) {
	target, _ := input["target_agent"].(string)
	if target == "" {
		return "", fmt.Errorf("handoff: target_agent is required")
	}

	context, _ := input["context"].(string)

	// Prefer HANDOFF: prefix format for simple parsing.
	return fmt.Sprintf("%s%s:%s", handoffPrefix, target, context), nil
}

