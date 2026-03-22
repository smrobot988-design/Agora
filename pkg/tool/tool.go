package tool

import (
	"context"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

// Tool is the interface that all tools must implement.
type Tool interface {
	// Definition returns the tool's schema definition for the LLM.
	Definition() schema.ToolDefinition

	// Execute runs the tool with the given input and returns a string result.
	// On failure, return an error; the agent loop will inject it as an error tool_result.
	Execute(ctx context.Context, input map[string]interface{}) (string, error)
}

// Func wraps a plain function as a Tool.
// This is a convenience for simple tools that don't need a struct. 适配器模式
type Func struct {
	Def     schema.ToolDefinition
	Handler func(ctx context.Context, input map[string]interface{}) (string, error)
}

// Definition returns the tool definition.
func (f *Func) Definition() schema.ToolDefinition { return f.Def }

// Execute calls the handler function.
func (f *Func) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	return f.Handler(ctx, input)
}
