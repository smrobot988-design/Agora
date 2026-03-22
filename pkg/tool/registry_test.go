package tool

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r.Len() != 0 {
		t.Fatalf("expected 0, got %d", r.Len())
	}
}

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(newEchoFunc()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tool, ok := r.Get("echo")
	if !ok {
		t.Fatal("expected to find 'echo' tool")
	}
	if tool.Definition().Name != "echo" {
		t.Fatalf("expected 'echo', got %q", tool.Definition().Name)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(newEchoFunc()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := r.Register(newEchoFunc())
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestRegisterEmptyName(t *testing.T) {
	r := NewRegistry()
	err := r.Register(&Func{
		Def:     schema.ToolDefinition{Name: ""},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) { return "", nil },
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestDefinitions(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(newEchoFunc())
	_ = r.Register(&Func{
		Def: schema.ToolDefinition{Name: "greet", Description: "Say hello"},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			return "hello", nil
		},
	})
	defs := r.Definitions()
	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["echo"] || !names["greet"] {
		t.Fatalf("expected echo and greet, got %v", names)
	}
}

func TestRegistryConcurrency(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		i := i
		go func() {
			defer wg.Done()
			_ = r.Register(&Func{
				Def: schema.ToolDefinition{Name: fmt.Sprintf("tool_%d", i)},
				Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
					return "", nil
				},
			})
		}()
		go func() {
			defer wg.Done()
			r.Get(fmt.Sprintf("tool_%d", i))
		}()
	}
	wg.Wait()
	if r.Len() != 100 {
		t.Fatalf("expected 100 tools, got %d", r.Len())
	}
}
