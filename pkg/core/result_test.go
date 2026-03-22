package core

import "testing"

func TestResultTotalTokens(t *testing.T) {
	r := &Result{TotalInputTokens: 150, TotalOutputTokens: 50}
	if r.TotalTokens() != 200 {
		t.Fatalf("expected 200, got %d", r.TotalTokens())
	}
}
