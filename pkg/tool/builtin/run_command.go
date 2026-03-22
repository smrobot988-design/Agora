package builtin

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

const defaultTimeout = 30           // seconds
const defaultMaxOutput = 100 * 1024 // 100KB

// RunCommand executes a shell command and returns its output.
type RunCommand struct {
	defaultTimeout int    // default timeout in seconds
	maxOutput      int    // maximum output size in bytes
	workDir        string // working directory
}

// RunCommandOption configures RunCommand.
type RunCommandOption func(*RunCommand)

// WithDefaultTimeout sets the default timeout for command execution.
func WithDefaultTimeout(seconds int) RunCommandOption {
	return func(r *RunCommand) {
		r.defaultTimeout = seconds
	}
}

// WithMaxOutput sets the maximum output size in bytes.
func WithMaxOutput(bytes int) RunCommandOption {
	return func(r *RunCommand) {
		r.maxOutput = bytes
	}
}

// WithWorkDir sets the working directory for command execution.
func WithWorkDir(dir string) RunCommandOption {
	return func(r *RunCommand) {
		r.workDir = dir
	}
}

// NewRunCommand creates a RunCommand tool.
func NewRunCommand(opts ...RunCommandOption) *RunCommand {
	r := &RunCommand{
		defaultTimeout: defaultTimeout,
		maxOutput:      defaultMaxOutput,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Definition returns the tool definition for the LLM.
func (r *RunCommand) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        "run_command",
		Description: "Execute a shell command and return its output (stdout and stderr combined). On Windows uses cmd /c, on Unix uses sh -c.",
		InputSchema: schema.PropertySchema{
			Properties: map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Timeout in seconds. Defaults to 30.",
				},
			},
			Required: []string{"command"},
		},
	}
}

// Execute runs the shell command and returns its output.
func (r *RunCommand) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	command, ok := stringFromInput(input, "command")
	if !ok || command == "" {
		return "", fmt.Errorf("run command: command is required")
	}

	timeout := intFromInput(input, "timeout", r.defaultTimeout)

	childCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(childCtx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(childCtx, "sh", "-c", command)
	}

	if r.workDir != "" {
		cmd.Dir = r.workDir
	}

	output, err := cmd.CombinedOutput()

	if err != nil {
		if childCtx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("%s\n[error: timeout after %d seconds]", r.truncate(output), timeout), nil
		}
		if childCtx.Err() == context.Canceled {
			return "", fmt.Errorf("run command: %w", childCtx.Err())
		}
		// Non-zero exit: include output + exit status as a successful result
		return fmt.Sprintf("%s\n[exit status: %s]", r.truncate(output), err.Error()), nil
	}

	return r.truncate(output), nil
}

// truncate truncates output if it exceeds maxOutput and appends a notice.
func (r *RunCommand) truncate(output []byte) string {
	if len(output) <= r.maxOutput {
		return string(output)
	}
	truncated := output[:r.maxOutput]
	return string(truncated) + fmt.Sprintf("\n... [output truncated, %d bytes total]", len(output))
}
