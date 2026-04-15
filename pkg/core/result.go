package core

// Result is returned from Agent.Run with the final output and execution metadata.
type Result struct {
	// Text is the final text response from the LLM.
	Text string

	// ReasoningText is the final reasoning content from the LLM.
	ReasoningText string

	// TotalInputTokens is the cumulative input tokens across all LLM calls.
	TotalInputTokens int

	// TotalOutputTokens is the cumulative output tokens across all LLM calls.
	TotalOutputTokens int

	// Turns is the number of LLM call iterations (including tool use rounds).
	Turns int
}

// TotalTokens returns the sum of input and output tokens.
func (r *Result) TotalTokens() int {
	return r.TotalInputTokens + r.TotalOutputTokens
}
