package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/smrobot988-design/Agora/pkg/core"
	"github.com/smrobot988-design/Agora/pkg/schema"
	"github.com/smrobot988-design/Agora/pkg/tool"
)

// handoffPrefix is the structured signal prefix that HandoffTool returns.
// The SwarmRunner parses result text for this prefix to detect handoffs.
const handoffPrefix = "HANDOFF:"

// SwarmRunner implements the Handoff/Swarm pattern.
// Agents can transfer control to other agents by calling a HandoffTool.
// Unlike Supervisor (where control returns to the coordinator), in Swarm
// the original agent exits and the target agent fully takes over.
//
// SwarmRunner implements Runner (not Strategizer) because it manages
// its own runner selection loop rather than being driven by Orchestrator.
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
//
//   - name: identifier for this swarm.
//   - entryPoint: name of the initial runner to execute.
//   - runners: map of runner name → Runner instance.
//   - opts: optional configuration.
//
// Each runner's Agent should have a HandoffTool registered in its tool registry
// so the LLM can signal handoffs.
func NewSwarmRunner(name, entryPoint string, runners map[string]Runner, opts ...SwarmOption) *SwarmRunner {
	s := &SwarmRunner{
		name:        name,
		runners:     runners,
		entryPoint:  entryPoint,
		maxHandoffs: 10,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run executes the swarm starting from the entry point.
// On each iteration, if the current agent's result contains a handoff signal,
// control transfers to the target agent. Otherwise, the result is returned.
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

		// Check for handoff signal.
		target, message, isHandoff := parseHandoff(result.Text)
		if !isHandoff {
			// Terminal result — no handoff, return.
			aggregated.Text = result.Text
			slog.Info("swarm completed",
				"swarm", s.name,
				"final_agent", current,
				"total_handoffs", handoff,
			)
			return aggregated, nil
		}

		slog.Info("swarm handoff",
			"from", current,
			"to", target,
		)

		current = target
		currentInput = message
	}

	return nil, fmt.Errorf("swarm %s: max handoffs (%d) exceeded", s.name, s.maxHandoffs)
}

// Name returns the swarm's name.
func (s *SwarmRunner) Name() string { return s.name }

// parseHandoff checks if the text starts with the handoff prefix
// and extracts the target agent name and context message.
// Format: "HANDOFF:<target>:<message>"
func parseHandoff(text string) (target, message string, isHandoff bool) {
	if !strings.HasPrefix(text, handoffPrefix) {
		return "", "", false
	}

	rest := text[len(handoffPrefix):]
	idx := strings.Index(rest, ":")
	if idx < 0 {
		// "HANDOFF:target" with no message.
		return rest, "", true
	}

	return rest[:idx], rest[idx+1:], true
}

// HandoffTool is a tool that agents in a swarm call to signal a handoff.
// When executed, it returns a structured string that SwarmRunner parses
// to determine the next agent to run.
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

// Execute returns a structured handoff signal that SwarmRunner parses.
func (t *HandoffTool) Execute(_ context.Context, input map[string]interface{}) (string, error) {
	target, _ := input["target_agent"].(string)
	if target == "" {
		return "", fmt.Errorf("handoff: target_agent is required")
	}

	message, _ := input["context"].(string)
	return fmt.Sprintf("%s%s:%s", handoffPrefix, target, message), nil
}
