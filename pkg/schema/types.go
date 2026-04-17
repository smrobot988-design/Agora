package schema

// ValidationIssue describes one schema validation failure for a tool call.
type ValidationIssue struct {
	Path     string `json:"path"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

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
	ID               string                 `json:"id,omitempty"`
	Name             string                 `json:"name,omitempty"`
	Input            map[string]interface{} `json:"input,omitempty"`
	RawArguments     string                 `json:"raw_arguments,omitempty"`
	ParseError       string                 `json:"parse_error,omitempty"`
	ValidationIssues []ValidationIssue      `json:"validation_issues,omitempty"`
}
