package core

import (
	"fmt"
	"sync"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/memory/store"
	"github.com/smrobot988-design/Agora/pkg/memory/trimmer"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

// mustLen is a test helper that calls Len and fails on error.
func mustLen(t *testing.T, m *Memory) int {
	t.Helper()
	n, err := m.Len()
	if err != nil {
		t.Fatalf("unexpected Len error: %v", err)
	}
	return n
}

// mustMessages is a test helper that calls Messages and fails on error.
func mustMessages(t *testing.T, m *Memory) []llm.Message {
	t.Helper()
	msgs, err := m.Messages()
	if err != nil {
		t.Fatalf("unexpected Messages error: %v", err)
	}
	return msgs
}

func TestNewMemoryDefaults(t *testing.T) {
	m := NewMemory()
	if mustLen(t, m) != 0 {
		t.Fatalf("expected 0 messages, got %d", mustLen(t, m))
	}
	if m.SystemPrompt() != "" {
		t.Fatalf("expected empty system prompt, got %q", m.SystemPrompt())
	}
}

func TestNewMemoryWithOptions(t *testing.T) {
	m := NewMemory(
		WithSystemPrompt("You are helpful."),
		WithTrimmer(&trimmer.SlidingWindow{MaxMessages: 10}),
	)
	if m.SystemPrompt() != "You are helpful." {
		t.Fatalf("expected system prompt, got %q", m.SystemPrompt())
	}
}

func TestNewMemoryWithStore(t *testing.T) {
	s := store.NewInMemory()
	m := NewMemory(WithStore(s))
	if err := m.AddUserMessage("hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	n, _ := s.Len()
	if n != 1 {
		t.Fatalf("expected 1 message in store, got %d", n)
	}
}

func TestAddUserMessage(t *testing.T) {
	m := NewMemory()
	if err := m.AddUserMessage("hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := mustMessages(t, m)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != llm.RoleUser {
		t.Fatalf("expected user role, got %s", msgs[0].Role)
	}
	if msgs[0].Content[0].Text != "hello" {
		t.Fatalf("expected 'hello', got %q", msgs[0].Content[0].Text)
	}
}

func TestAddAssistantMessage(t *testing.T) {
	m := NewMemory()
	if err := m.AddAssistantMessage("hi there"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := mustMessages(t, m)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != llm.RoleAssistant {
		t.Fatalf("expected assistant role, got %s", msgs[0].Role)
	}
	if msgs[0].Content[0].Text != "hi there" {
		t.Fatalf("expected 'hi there', got %q", msgs[0].Content[0].Text)
	}
}

func TestAddAssistantResponse(t *testing.T) {
	m := NewMemory()
	resp := &llm.Response{
		Text: "Let me check the weather.",
		ToolCalls: []schema.ToolCall{
			{ID: "call_1", Name: "get_weather", Input: map[string]interface{}{"city": "Tokyo"}},
		},
	}
	if err := m.AddAssistantResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := mustMessages(t, m)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != llm.RoleAssistant {
		t.Fatalf("expected assistant role, got %s", msg.Role)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 blocks (text + tool_use), got %d", len(msg.Content))
	}
	if msg.Content[0].Type != llm.BlockText || msg.Content[0].Text != "Let me check the weather." {
		t.Fatal("expected text block with correct content")
	}
	if msg.Content[1].Type != llm.BlockToolUse || msg.Content[1].ToolCall.Name != "get_weather" {
		t.Fatal("expected tool_use block with get_weather")
	}
}

func TestAddAssistantResponseTextOnly(t *testing.T) {
	m := NewMemory()
	resp := &llm.Response{Text: "Hello!"}
	if err := m.AddAssistantResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := mustMessages(t, m)
	if len(msgs[0].Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(msgs[0].Content))
	}
	if msgs[0].Content[0].Type != llm.BlockText {
		t.Fatalf("expected text block, got %s", msgs[0].Content[0].Type)
	}
}

func TestAddAssistantResponseIgnoresReasoningText(t *testing.T) {
	m := NewMemory()
	resp := &llm.Response{
		Text:          "Hello!",
		ReasoningText: "internal reasoning",
	}
	if err := m.AddAssistantResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := mustMessages(t, m)
	if len(msgs[0].Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(msgs[0].Content))
	}
	if msgs[0].Content[0].Text != "Hello!" {
		t.Fatalf("expected final text only, got %q", msgs[0].Content[0].Text)
	}
}

func TestAddAssistantResponseToolCallsOnly(t *testing.T) {
	m := NewMemory()
	resp := &llm.Response{
		ToolCalls: []schema.ToolCall{
			{ID: "call_1", Name: "read_file", Input: map[string]interface{}{"path": "/tmp/test"}},
		},
	}
	if err := m.AddAssistantResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := mustMessages(t, m)
	if len(msgs[0].Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(msgs[0].Content))
	}
	if msgs[0].Content[0].Type != llm.BlockToolUse {
		t.Fatalf("expected tool_use block, got %s", msgs[0].Content[0].Type)
	}
}

func TestAddToolResult(t *testing.T) {
	m := NewMemory()
	if err := m.AddToolResult([]llm.ToolResult{
		{ToolCallID: "call_1", Content: `{"temp": "20°C"}`, IsError: false},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := mustMessages(t, m)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != llm.RoleTool {
		t.Fatalf("expected tool role, got %s", msgs[0].Role)
	}
	if msgs[0].Content[0].ToolCallID != "call_1" {
		t.Fatalf("expected tool_call_id 'call_1', got %s", msgs[0].Content[0].ToolCallID)
	}
}

func TestMessagesReturnsCopy(t *testing.T) {
	m := NewMemory()
	_ = m.AddUserMessage("hello")
	msgs := mustMessages(t, m)
	msgs[0] = llm.NewTextMessage(llm.RoleAssistant, "tampered")
	original := mustMessages(t, m)
	if original[0].Role != llm.RoleUser {
		t.Fatal("modifying returned slice should not affect internal state")
	}
}

func TestSystemPrompt(t *testing.T) {
	m := NewMemory()
	m.SetSystemPrompt("You are a coding assistant.")
	if m.SystemPrompt() != "You are a coding assistant." {
		t.Fatalf("expected system prompt, got %q", m.SystemPrompt())
	}
}

func TestClear(t *testing.T) {
	m := NewMemory(WithSystemPrompt("keep me"))
	_ = m.AddUserMessage("hello")
	_ = m.AddAssistantMessage("hi")
	if err := m.Clear(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mustLen(t, m) != 0 {
		t.Fatalf("expected 0 messages after clear, got %d", mustLen(t, m))
	}
	if m.SystemPrompt() != "keep me" {
		t.Fatal("clear should preserve system prompt")
	}
}

func TestReset(t *testing.T) {
	m := NewMemory(WithSystemPrompt("remove me"))
	_ = m.AddUserMessage("hello")
	if err := m.Reset(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mustLen(t, m) != 0 {
		t.Fatalf("expected 0 messages after reset, got %d", mustLen(t, m))
	}
	if m.SystemPrompt() != "" {
		t.Fatal("reset should clear system prompt")
	}
}

func TestConversationFlow(t *testing.T) {
	m := NewMemory(WithSystemPrompt("You are helpful."))
	_ = m.AddUserMessage("What's the weather?")
	_ = m.AddAssistantResponse(&llm.Response{
		StopReason: llm.StopReasonToolUse,
		ToolCalls:  []schema.ToolCall{{ID: "c1", Name: "get_weather", Input: map[string]interface{}{"city": "SF"}}},
	})
	_ = m.AddToolResult([]llm.ToolResult{{ToolCallID: "c1", Content: `{"temp":"18°C"}`}})
	_ = m.AddAssistantResponse(&llm.Response{
		StopReason: llm.StopReasonEndTurn,
		Text:       "It's 18°C in SF.",
	})

	msgs := mustMessages(t, m)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	expectedRoles := []llm.Role{llm.RoleUser, llm.RoleAssistant, llm.RoleTool, llm.RoleAssistant}
	for i, role := range expectedRoles {
		if msgs[i].Role != role {
			t.Fatalf("message %d: expected role %s, got %s", i, role, msgs[i].Role)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewMemory()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = m.AddUserMessage("hello")
		}()
		go func() {
			defer wg.Done()
			_, _ = m.Messages()
		}()
	}
	wg.Wait()
	if mustLen(t, m) != 100 {
		t.Fatalf("expected 100 messages, got %d", mustLen(t, m))
	}
}

// failingStore is a test helper that always returns errors.
type failingStore struct{}

func (s *failingStore) Append(msg llm.Message) error     { return fmt.Errorf("append failed") }
func (s *failingStore) Messages() ([]llm.Message, error) { return nil, fmt.Errorf("messages failed") }
func (s *failingStore) Len() (int, error)                { return 0, fmt.Errorf("len failed") }
func (s *failingStore) Clear() error                     { return fmt.Errorf("clear failed") }

func TestStoreErrorPropagation(t *testing.T) {
	m := NewMemory(WithStore(&failingStore{}))

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
