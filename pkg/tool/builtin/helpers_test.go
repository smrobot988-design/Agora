package builtin

import (
	"testing"
)

func TestStringFromInputPresent(t *testing.T) {
	input := map[string]interface{}{"name": "test_value"}
	got, ok := stringFromInput(input, "name")
	if !ok {
		t.Error("stringFromInput returned ok=false, want true")
	}
	if got != "test_value" {
		t.Errorf("stringFromInput returned %q, want %q", got, "test_value")
	}
}

func TestStringFromInputMissing(t *testing.T) {
	input := map[string]interface{}{"other": "value"}
	got, ok := stringFromInput(input, "name")
	if ok {
		t.Error("stringFromInput returned ok=true, want false")
	}
	if got != "" {
		t.Errorf("stringFromInput returned %q, want %q", got, "")
	}
}

func TestIntFromInputFloat64(t *testing.T) {
	input := map[string]interface{}{"count": float64(42)}
	got := intFromInput(input, "count", 0)
	if got != 42 {
		t.Errorf("intFromInput returned %d, want 42", got)
	}
}

func TestIntFromInputMissing(t *testing.T) {
	input := map[string]interface{}{"other": float64(10)}
	got := intFromInput(input, "count", 99)
	if got != 99 {
		t.Errorf("intFromInput returned %d, want 99", got)
	}
}

func TestIntFromInputWrongType(t *testing.T) {
	input := map[string]interface{}{"count": "not_a_number"}
	got := intFromInput(input, "count", 77)
	if got != 77 {
		t.Errorf("intFromInput returned %d, want 77", got)
	}
}
