package store

import "github.com/smrobot988-design/Agora/pkg/llm"

// Store defines the storage backend for conversation messages.
// Implement this interface to persist messages to a database, Redis, file, etc.
// The default implementation is InMemory (in-memory).
//
// Store implementations do not need to be thread-safe; Memory wraps
// all Store calls within its own mutex.
type Store interface {
	// Append adds a message to the store.
	Append(msg llm.Message) error

	// Messages returns all stored messages.
	Messages() ([]llm.Message, error)

	// Len returns the number of stored messages.
	Len() (int, error)

	// Clear removes all stored messages.
	Clear() error
}

// InMemory is an in-memory Store implementation. This is the default.
type InMemory struct {
	messages []llm.Message
}

// NewInMemory creates a new in-memory store.
func NewInMemory() *InMemory {
	return &InMemory{}
}

// Append adds a message to the in-memory store.
func (s *InMemory) Append(msg llm.Message) error {
	s.messages = append(s.messages, msg)
	return nil
}

// Messages returns all stored messages.
func (s *InMemory) Messages() ([]llm.Message, error) {
	return s.messages, nil
}

// Len returns the number of stored messages.
func (s *InMemory) Len() (int, error) {
	return len(s.messages), nil
}

// Clear removes all stored messages.
func (s *InMemory) Clear() error {
	s.messages = nil
	return nil
}
