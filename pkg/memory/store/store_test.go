package store

import (
	"testing"

	"github.com/smrobot988-design/Agora/pkg/llm"
)

func TestInMemory(t *testing.T) {
	s := NewInMemory()
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
