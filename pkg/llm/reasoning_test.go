package llm

import "testing"

func TestReasoningParserThinkTag(t *testing.T) {
	tests := []struct {
		name      string
		chunks    []string
		text      string
		reasoning string
	}{
		{
			name:   "plain text",
			chunks: []string{"hello"},
			text:   "hello",
		},
		{
			name:      "complete think tag",
			chunks:    []string{"<think>hidden</think>visible"},
			text:      "visible",
			reasoning: "hidden",
		},
		{
			name:      "split tags across chunks",
			chunks:    []string{"pre <th", "ink>hidden</thi", "nk> post"},
			text:      "pre  post",
			reasoning: "hidden",
		},
		{
			name:      "multiple think blocks",
			chunks:    []string{"a<think>b</think>c<think>d</think>e"},
			text:      "ace",
			reasoning: "bd",
		},
		{
			name:      "unclosed think block",
			chunks:    []string{"a<think>b"},
			text:      "a",
			reasoning: "b",
		},
		{
			name:   "similar tag is plain text",
			chunks: []string{"<thinking>keep</thinking>"},
			text:   "<thinking>keep</thinking>",
		},
		{
			name:   "partial tag fallback",
			chunks: []string{"abc<th", "x"},
			text:   "abc<thx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := newReasoningParser(ReasoningModeThinkTag)
			var text, reasoning string
			for _, chunk := range tt.chunks {
				appendSegments(&text, &reasoning, parser.Consume(chunk))
			}
			appendSegments(&text, &reasoning, parser.Flush())

			if text != tt.text {
				t.Fatalf("expected text %q, got %q", tt.text, text)
			}
			if reasoning != tt.reasoning {
				t.Fatalf("expected reasoning %q, got %q", tt.reasoning, reasoning)
			}
		})
	}
}

func TestReasoningParserNone(t *testing.T) {
	parser := newReasoningParser(ReasoningModeNone)
	var text, reasoning string
	appendSegments(&text, &reasoning, parser.Consume("<think>plain</think>"))
	appendSegments(&text, &reasoning, parser.Flush())

	if text != "<think>plain</think>" {
		t.Fatalf("expected plain text, got %q", text)
	}
	if reasoning != "" {
		t.Fatalf("expected no reasoning, got %q", reasoning)
	}
}

func appendSegments(text, reasoning *string, segments []contentSegment) {
	for _, segment := range segments {
		switch segment.kind {
		case contentKindReasoning:
			*reasoning += segment.text
		default:
			*text += segment.text
		}
	}
}
