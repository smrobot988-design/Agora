package llm

// ResponseFormatType identifies provider-neutral structured response formats.
type ResponseFormatType string

const (
	// ResponseFormatJSONObject asks providers that support JSON Output / JSON
	// mode to return a valid JSON object as the final text content.
	ResponseFormatJSONObject ResponseFormatType = "json_object"
)

// ResponseFormat configures provider-side structured text output.
type ResponseFormat struct {
	Type ResponseFormatType `json:"type,omitempty"`
}
