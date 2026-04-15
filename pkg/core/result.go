package core

import "github.com/smrobot988-design/Agora/pkg/llm"

// Result is returned from Agent.Run with the final output and execution metadata.
type Result struct {
	// Text is the final text response from the LLM.
	Text string

	// ReasoningText is the final reasoning content from the LLM.
	ReasoningText string

	// AppliedReasoning describes how Agora actually handled provider reasoning.
	AppliedReasoning *llm.AppliedReasoning

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
