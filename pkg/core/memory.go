package core

import (
	"fmt"
	"sync"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/memory/store"
	"github.com/smrobot988-design/Agora/pkg/memory/trimmer"
)

// Memory manages conversation history for an Agent.
type Memory struct {
	mu           sync.RWMutex
	systemPrompt string
	store        store.Store
	trimmer      trimmer.Trimmer
}

// MemoryOption configures Memory.
type MemoryOption func(*Memory)

// WithSystemPrompt sets the initial system prompt.
func WithSystemPrompt(prompt string) MemoryOption {
	return func(m *Memory) { m.systemPrompt = prompt }
}

// WithTrimmer sets the context window management strategy.
func WithTrimmer(t trimmer.Trimmer) MemoryOption {
	return func(m *Memory) { m.trimmer = t }
}

// WithStore sets the storage backend for messages.
// Default is store.InMemory (in-memory). Implement the store.Store interface
// to persist messages to a database, Redis, file, etc.
func WithStore(s store.Store) MemoryOption {
	return func(m *Memory) { m.store = s }
}

// NewMemory creates a new Memory with the given options.
// Defaults to store.InMemory and trimmer.NoOp if not provided.
func NewMemory(opts ...MemoryOption) *Memory {
	m := &Memory{
		store:   store.NewInMemory(),
		trimmer: &trimmer.NoOp{},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// AddUserMessage appends a user text message to the conversation.
func (m *Memory) AddUserMessage(text string) error {
	return m.appendMsg(llm.NewTextMessage(llm.RoleUser, text))
}

// AddAssistantMessage appends a simple text-only assistant message.
// For adding full LLM responses (with tool calls), use AddAssistantResponse.
func (m *Memory) AddAssistantMessage(text string) error {
	return m.appendMsg(llm.NewTextMessage(llm.RoleAssistant, text))
}

// AddAssistantResponse appends an assistant message built from an LLM Response.
// It constructs ContentBlocks from the Response's text and tool calls,
// preserving the structure needed for follow-up API calls.
func (m *Memory) AddAssistantResponse(resp *llm.Response) error {
	var blocks []llm.ContentBlock
	if resp.Text != "" {
		blocks = append(blocks, llm.ContentBlock{Type: llm.BlockText, Text: resp.Text})
	}
	for _, tc := range resp.ToolCalls {
		blocks = append(blocks, llm.ContentBlock{
			Type:     llm.BlockToolUse,
			ToolCall: tc,
		})
	}
	return m.appendMsg(llm.Message{Role: llm.RoleAssistant, Content: blocks})
}

// AddToolResult appends tool execution results as a tool-role message.
func (m *Memory) AddToolResult(results []llm.ToolResult) error {
	return m.appendMsg(llm.NewToolResultMessage(results))
}

// AddMessage appends an arbitrary message. Use the typed methods
// (AddUserMessage, AddAssistantResponse, AddToolResult) when possible.
func (m *Memory) AddMessage(msg llm.Message) error {
	return m.appendMsg(msg)
}

// Messages returns a copy of the conversation messages with trimming applied.
// This is the method to use when sending messages to Provider.Chat().
func (m *Memory) Messages() ([]llm.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	msgs, err := m.store.Messages()
	if err != nil {
		return nil, fmt.Errorf("store messages: %w", err)
	}

	trimmed := m.trimmer.Trim(msgs)
	result := make([]llm.Message, len(trimmed))
	copy(result, trimmed)
	return result, nil
}

// AllMessages returns a copy of all messages without trimming.
// Useful for tracing and debugging.
func (m *Memory) AllMessages() ([]llm.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	msgs, err := m.store.Messages()
	if err != nil {
		return nil, fmt.Errorf("store messages: %w", err)
	}

	result := make([]llm.Message, len(msgs))
	copy(result, msgs)
	return result, nil
}

// SystemPrompt returns the current system prompt.
func (m *Memory) SystemPrompt() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemPrompt
}

// SetSystemPrompt updates the system prompt.
func (m *Memory) SetSystemPrompt(prompt string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemPrompt = prompt
}

// Len returns the number of messages in history (before trimming).
func (m *Memory) Len() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.store.Len()
}

// Clear removes all messages but preserves the system prompt.
func (m *Memory) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.Clear()
}

// Reset removes all messages AND the system prompt.
func (m *Memory) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemPrompt = ""
	return m.store.Clear()
}

// appendMsg is the internal method that adds a message (mutex locked).
func (m *Memory) appendMsg(msg llm.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.Append(msg)
}
