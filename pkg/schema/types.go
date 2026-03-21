package schema

// ToolDefinition describes a tool that an LLM can invoke.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema PropertySchema `json:"input_schema"`
}

// PropertySchema represents the JSON Schema properties for a tool's input.
type PropertySchema struct {
	Properties map[string]interface{} `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// ToolCall represents a tool invocation requested by an LLM.
type ToolCall struct {
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}
