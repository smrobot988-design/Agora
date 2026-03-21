package memory

import (
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/llm"
)

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore()
	if n, _ := s.Len(); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	_ = s.Append(llm.NewTextMessage(llm.RoleUser, "hello"))
	_ = s.Append(llm.NewTextMessage(llm.RoleAssistant, "hi"))
	if n, _ := s.Len(); n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
	msgs, _ := s.Messages()
	if msgs[0].Content[0].Text != "hello" {
		t.Fatalf("expected 'hello', got %q", msgs[0].Content[0].Text)
	}
	_ = s.Clear()
	if n, _ := s.Len(); n != 0 {
		t.Fatalf("expected 0 after clear, got %d", n)
	}
}

// failingStore is a test helper that always returns errors.
type failingStore struct{}

func (s *failingStore) Append(msg llm.Message) error    { return fmt.Errorf("append failed") }
func (s *failingStore) Messages() ([]llm.Message, error) { return nil, fmt.Errorf("messages failed") }
func (s *failingStore) Len() (int, error)                { return 0, fmt.Errorf("len failed") }
func (s *failingStore) Clear() error                     { return fmt.Errorf("clear failed") }

func TestStoreErrorPropagation(t *testing.T) {
	m := New(WithStore(&failingStore{}))

	if err := m.AddUserMessage("hello"); err == nil {
		t.Fatal("expected error from failing store")
	}
	if _, err := m.Messages(); err == nil {
		t.Fatal("expected error from failing store")
	}
	if _, err := m.AllMessages(); err == nil {
		t.Fatal("expected error from failing store")
	}
	if _, err := m.Len(); err == nil {
		t.Fatal("expected error from failing store")
	}
	if err := m.Clear(); err == nil {
		t.Fatal("expected error from failing store")
	}
	if err := m.Reset(); err == nil {
		t.Fatal("expected error from failing store")
	}
}
