package tool

import (
	"context"
	"fmt"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

func newEchoFunc() *Func {
	return &Func{
		Def: schema.ToolDefinition{
			Name:        "echo",
			Description: "Returns the input message",
			InputSchema: schema.PropertySchema{
				Properties: map[string]interface{}{
					"message": map[string]interface{}{
						"type":        "string",
						"description": "The message to echo back",
					},
				},
				Required: []string{"message"},
			},
		},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			msg, _ := input["message"].(string)
			return msg, nil
		},
	}
}

func TestFuncDefinition(t *testing.T) {
	f := newEchoFunc()
	def := f.Definition()
	if def.Name != "echo" {
		t.Fatalf("expected 'echo', got %q", def.Name)
	}
	if def.Description != "Returns the input message" {
		t.Fatalf("expected description, got %q", def.Description)
	}
	if len(def.InputSchema.Required) != 1 || def.InputSchema.Required[0] != "message" {
		t.Fatalf("expected required [message], got %v", def.InputSchema.Required)
	}
}

func TestFuncExecute(t *testing.T) {
	f := newEchoFunc()
	result, err := f.Execute(context.Background(), map[string]interface{}{"message": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got %q", result)
	}
}

func TestFuncExecuteError(t *testing.T) {
	f := &Func{
		Def: schema.ToolDefinition{Name: "fail"},
		Handler: func(ctx context.Context, input map[string]interface{}) (string, error) {
			return "", fmt.Errorf("something went wrong")
		},
	}
	_, err := f.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "something went wrong" {
		t.Fatalf("expected 'something went wrong', got %q", err.Error())
	}
}
