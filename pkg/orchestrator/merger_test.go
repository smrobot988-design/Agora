package orchestrator

import (
	"context"
	"testing"

	"github.com/smrobot988-design/Agora/pkg/core"
)

func TestConcatMergerEmpty(t *testing.T) {
	m := &ConcatMerger{Separator: "\n---\n"}
	result, err := m.Merge(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "" {
		t.Errorf("got %q, want empty", result.Text)
	}
}

func TestConcatMerger(t *testing.T) {
	m := &ConcatMerger{Separator: " | "}
	results := []*core.Result{
		{Text: "alpha", TotalInputTokens: 10, TotalOutputTokens: 5, Turns: 1},
		{Text: "beta", TotalInputTokens: 20, TotalOutputTokens: 10, Turns: 3},
		{Text: "gamma", TotalInputTokens: 15, TotalOutputTokens: 8, Turns: 2},
	}

	merged, err := m.Merge(context.Background(), results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Text != "alpha | beta | gamma" {
		t.Errorf("got %q, want %q", merged.Text, "alpha | beta | gamma")
	}
	if merged.TotalInputTokens != 45 {
		t.Errorf("input tokens = %d, want 45", merged.TotalInputTokens)
	}
	if merged.TotalOutputTokens != 23 {
		t.Errorf("output tokens = %d, want 23", merged.TotalOutputTokens)
	}
	if merged.Turns != 3 {
		t.Errorf("turns = %d, want 3 (max)", merged.Turns)
	}
}

func TestConcatMergerSkipsNil(t *testing.T) {
	m := &ConcatMerger{Separator: ","}
	results := []*core.Result{
		{Text: "a"},
		nil,
		{Text: "b"},
	}
	merged, err := m.Merge(context.Background(), results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Text != "a,b" {
		t.Errorf("got %q, want %q", merged.Text, "a,b")
	}
}

func TestFirstSuccessMerger(t *testing.T) {
	m := &FirstSuccessMerger{}
	results := []*core.Result{
		nil,
		{Text: "winner"},
		{Text: "loser"},
	}
	merged, err := m.Merge(context.Background(), results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Text != "winner" {
		t.Errorf("got %q, want %q", merged.Text, "winner")
	}
}

func TestFirstSuccessMergerAllNil(t *testing.T) {
	m := &FirstSuccessMerger{}
	_, err := m.Merge(context.Background(), []*core.Result{nil, nil})
	if err == nil {
		t.Error("expected error for all-nil results")
	}
}
