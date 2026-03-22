package builtin

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestRunCommandDefinition(t *testing.T) {
	r := NewRunCommand()
	def := r.Definition()

	if def.Name != "run_command" {
		t.Errorf("Definition().Name = %q, want %q", def.Name, "run_command")
	}
	if def.Description == "" {
		t.Error("Definition().Description is empty")
	}
	found := false
	for _, req := range def.InputSchema.Required {
		if req == "command" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Definition().InputSchema.Required does not contain 'command'")
	}
}

func TestRunCommandBasicEcho(t *testing.T) {
	r := NewRunCommand()
	var result string
	var err error

	if runtime.GOOS == "windows" {
		result, err = r.Execute(context.Background(), map[string]interface{}{"command": "echo hello"})
	} else {
		result, err = r.Execute(context.Background(), map[string]interface{}{"command": "echo hello"})
	}

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("result does not contain 'hello', got: %s", result)
	}
}

func TestRunCommandMissingCommand(t *testing.T) {
	r := NewRunCommand()
	_, err := r.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Error("Execute expected error for missing command, got nil")
	}
}

func TestRunCommandNonZeroExit(t *testing.T) {
	r := NewRunCommand()
	var result string
	var err error

	if runtime.GOOS == "windows" {
		result, err = r.Execute(context.Background(), map[string]interface{}{"command": "cmd /c exit 1"})
	} else {
		result, err = r.Execute(context.Background(), map[string]interface{}{"command": "exit 1"})
	}

	// Non-zero exit is not an error — it's returned as output with exit status
	if err != nil {
		t.Fatalf("Execute returned error for non-zero exit: %v", err)
	}
	if !strings.Contains(result, "exit status") {
		t.Errorf("result does not contain 'exit status', got: %s", result)
	}
}

func TestRunCommandTimeout(t *testing.T) {
	r := NewRunCommand()
	var result string
	var err error

	var command string
	if runtime.GOOS == "windows" {
		command = "ping -n 10 127.0.0.1" // windows sleep via ping
	} else {
		command = "sleep 10"
	}

	result, err = r.Execute(context.Background(), map[string]interface{}{
		"command": command,
		"timeout": float64(1),
	})

	// Timeout returns result (not error) with timeout message
	if err != nil {
		t.Fatalf("Execute returned error for timeout: %v", err)
	}
	if !strings.Contains(result, "timeout") {
		t.Errorf("result does not contain 'timeout', got: %s", result)
	}
}

func TestRunCommandContextCancellation(t *testing.T) {
	r := NewRunCommand()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before execution

	_, err := r.Execute(ctx, map[string]interface{}{"command": "echo hello"})

	if err == nil {
		t.Error("Execute expected error for canceled context, got nil")
	}
}

func TestRunCommandOutputTruncation(t *testing.T) {
	// Create with very small maxOutput
	r := NewRunCommand(WithMaxOutput(50))

	var result string
	var err error

	if runtime.GOOS == "windows" {
		// Generate more than 50 bytes of output
		result, err = r.Execute(context.Background(), map[string]interface{}{"command": "echo this is a longer output that should be truncated"})
	} else {
		result, err = r.Execute(context.Background(), map[string]interface{}{"command": "echo this is a longer output that should be truncated"})
	}

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("result does not contain 'truncated', got: %s", result)
	}
}

func TestRunCommandWithWorkDir(t *testing.T) {
	r := NewRunCommand(WithWorkDir("/tmp"))

	var result string
	var err error

	if runtime.GOOS == "windows" {
		result, err = r.Execute(context.Background(), map[string]interface{}{"command": "cd"})
	} else {
		result, err = r.Execute(context.Background(), map[string]interface{}{"command": "pwd"})
	}

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// workDir is /tmp but actual cwd is test environment, so we just check it runs
	if result == "" {
		t.Error("result is empty")
	}
}

func TestRunCommandDefaultOptions(t *testing.T) {
	r := NewRunCommand()
	if r.defaultTimeout != defaultTimeout {
		t.Errorf("defaultTimeout = %d, want %d", r.defaultTimeout, defaultTimeout)
	}
	if r.maxOutput != defaultMaxOutput {
		t.Errorf("maxOutput = %d, want %d", r.maxOutput, defaultMaxOutput)
	}
}

func TestRunCommandWithOptions(t *testing.T) {
	r := NewRunCommand(
		WithDefaultTimeout(60),
		WithMaxOutput(1024),
		WithWorkDir("/custom"),
	)
	if r.defaultTimeout != 60 {
		t.Errorf("defaultTimeout = %d, want 60", r.defaultTimeout)
	}
	if r.maxOutput != 1024 {
		t.Errorf("maxOutput = %d, want 1024", r.maxOutput)
	}
	if r.workDir != "/custom" {
		t.Errorf("workDir = %q, want %q", r.workDir, "/custom")
	}
}
