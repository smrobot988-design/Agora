package schema

import "testing"

func TestNormalizeToolInputCoercesAndPrunes(t *testing.T) {
	def := ToolDefinition{
		Name: "search",
		InputSchema: PropertySchema{
			Properties: map[string]interface{}{
				"query": map[string]interface{}{"type": "string"},
				"limit": map[string]interface{}{"type": "integer"},
				"exact": map[string]interface{}{"type": "boolean"},
			},
			Required: []string{"query"},
		},
	}

	normalized, issues := NormalizeToolInput(def, map[string]interface{}{
		"query": "golang",
		"limit": "3",
		"exact": "true",
		"noise": "drop-me",
	})
	if len(issues) > 0 {
		t.Fatalf("expected no validation issues, got %#v", issues)
	}
	if _, ok := normalized["noise"]; ok {
		t.Fatalf("expected unknown field to be pruned, got %#v", normalized)
	}
	if normalized["limit"] != float64(3) {
		t.Fatalf("expected limit to be coerced to number, got %#v", normalized["limit"])
	}
	if normalized["exact"] != true {
		t.Fatalf("expected exact to be coerced to bool, got %#v", normalized["exact"])
	}
}

func TestNormalizeToolInputReportsMissingRequired(t *testing.T) {
	def := ToolDefinition{
		Name: "search",
		InputSchema: PropertySchema{
			Properties: map[string]interface{}{
				"query": map[string]interface{}{"type": "string"},
			},
			Required: []string{"query"},
		},
	}

	_, issues := NormalizeToolInput(def, map[string]interface{}{})
	if len(issues) == 0 {
		t.Fatal("expected validation issues")
	}
	if issues[0].Code != "missing_required" {
		t.Fatalf("expected missing_required, got %#v", issues[0])
	}
}
