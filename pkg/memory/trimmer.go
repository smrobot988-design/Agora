package memory

import "github.com/smrobot988-design/Agora/pkg/llm"

// Trimmer defines a strategy for managing the context window.
// When the conversation grows too large, a Trimmer decides which
// messages to keep. Implement this interface to provide custom
// trimming strategies (e.g. summarization, importance-based, etc.).
type Trimmer interface {
	// Trim receives the full message slice and returns a trimmed version.
	// It must not modify the input slice.
	Trim(messages []llm.Message) []llm.Message
}

// NoOpTrimmer performs no trimming (unlimited history). This is the default.
type NoOpTrimmer struct{}

// Trim returns messages unchanged.
func (t *NoOpTrimmer) Trim(messages []llm.Message) []llm.Message {
	return messages
}

// SlidingWindowTrimmer keeps the most recent MaxMessages messages.
type SlidingWindowTrimmer struct {
	// MaxMessages is the maximum number of messages to keep.
	MaxMessages int
	// PreserveFirst keeps the first message (often the initial user task)
	// even when trimming. When true, the result is: [first] + [last N-1].
	PreserveFirst bool
}

// Trim keeps the most recent messages within the window size.
func (t *SlidingWindowTrimmer) Trim(messages []llm.Message) []llm.Message {
	if len(messages) <= t.MaxMessages {
		return messages
	}

	if t.PreserveFirst && t.MaxMessages > 1 && len(messages) > 1 {
		tail := messages[len(messages)-(t.MaxMessages-1):]
		result := make([]llm.Message, 0, t.MaxMessages)
		result = append(result, messages[0])
		result = append(result, tail...)
		return result
	}

	return messages[len(messages)-t.MaxMessages:]
}

// TokenBudgetTrimmer trims messages to fit within a token budget.
// It requires a token estimation function, which can be provided
// by the Agent from Provider.EstimateTokens.
type TokenBudgetTrimmer struct {
	// MaxTokens is the maximum token budget.
	MaxTokens int
	// EstimateTokens estimates the token count for a set of messages.
	EstimateTokens func(messages []llm.Message) int
}

// Trim drops the oldest messages (preserving the first) until under budget.
func (t *TokenBudgetTrimmer) Trim(messages []llm.Message) []llm.Message {
	if t.EstimateTokens == nil || t.EstimateTokens(messages) <= t.MaxTokens {
		return messages
	}

	// Drop oldest messages from the front until under budget.
	for start := 1; start < len(messages); start++ {
		candidate := messages[start:]
		if t.EstimateTokens(candidate) <= t.MaxTokens {
			return candidate
		}
	}

	// If even a single message exceeds budget, return just the last message.
	if len(messages) > 0 {
		return messages[len(messages)-1:]
	}
	return messages
}
