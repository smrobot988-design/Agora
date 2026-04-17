package llm

import "testing"

func TestParseToolArgumentsRepairsCommonJSONFormatting(t *testing.T) {
	input, parseError := parseToolArguments("```json\n{\"location\":\"Tokyo\",}\n```")
	if parseError != "" {
		t.Fatalf("expected repaired JSON, got parse error %q", parseError)
	}
	if input["location"] != "Tokyo" {
		t.Fatalf("expected location Tokyo, got %#v", input)
	}
}

func TestParseToolArgumentsReportsMalformedJSON(t *testing.T) {
	_, parseError := parseToolArguments("{location: Tokyo}")
	if parseError == "" {
		t.Fatal("expected parse error")
	}
}
