package llm

import "strings"

// ReasoningMode controls how provider output is classified into
// user-visible text vs. reasoning content.
//
// Deprecated: prefer using ReasoningParseMode in new APIs. ReasoningMode is kept
// as a backwards-compatible alias for the same concept.
type ReasoningMode string

const (
	ReasoningModeNone     ReasoningMode = "none"
	ReasoningModeThinkTag ReasoningMode = "think_tag"
	ReasoningModeNative   ReasoningMode = "native"
)

// ReasoningParseMode is the preferred name for configuring how raw provider
// output is split into visible text vs. reasoning content.
type ReasoningParseMode = ReasoningMode

const (
	ReasoningParseModeNone     = ReasoningModeNone
	ReasoningParseModeThinkTag = ReasoningModeThinkTag
	ReasoningParseModeNative   = ReasoningModeNative
)

type contentKind int

const (
	contentKindText contentKind = iota
	contentKindReasoning
)

type contentSegment struct {
	kind contentKind
	text string
}

type reasoningParser struct {
	mode        ReasoningMode
	inReasoning bool
	pending     string
}

func newReasoningParser(mode ReasoningMode) *reasoningParser {
	if mode == "" {
		mode = ReasoningModeNone
	}
	return &reasoningParser{mode: mode}
}

func classifyContent(mode ReasoningMode, content string) (text, reasoning string) {
	parser := newReasoningParser(mode)
	segments := parser.Consume(content)
	segments = append(segments, parser.Flush()...)
	for _, segment := range segments {
		switch segment.kind {
		case contentKindReasoning:
			reasoning += segment.text
		default:
			text += segment.text
		}
	}
	return text, reasoning
}

func (p *reasoningParser) Consume(chunk string) []contentSegment {
	if chunk == "" {
		return nil
	}
	if p.mode != ReasoningModeThinkTag {
		return []contentSegment{{kind: contentKindText, text: chunk}}
	}

	const openTag = "<think>"
	const closeTag = "</think>"

	buffer := p.pending + chunk
	p.pending = ""

	var segments []contentSegment
	cursor := 0
	for cursor < len(buffer) {
		target := openTag
		if p.inReasoning {
			target = closeTag
		}

		rel := strings.IndexByte(buffer[cursor:], '<')
		if rel < 0 {
			p.appendSegment(&segments, p.currentKind(), buffer[cursor:])
			break
		}

		idx := cursor + rel
		if idx > cursor {
			p.appendSegment(&segments, p.currentKind(), buffer[cursor:idx])
		}

		rest := buffer[idx:]
		if strings.HasPrefix(rest, target) {
			p.inReasoning = !p.inReasoning
			cursor = idx + len(target)
			continue
		}
		if len(rest) < len(target) && strings.HasPrefix(target, rest) {
			p.pending = rest
			break
		}

		p.appendSegment(&segments, p.currentKind(), buffer[idx:idx+1])
		cursor = idx + 1
	}

	return segments
}

func (p *reasoningParser) Flush() []contentSegment {
	if p.pending == "" {
		return nil
	}
	segment := contentSegment{
		kind: p.currentKind(),
		text: p.pending,
	}
	p.pending = ""
	return []contentSegment{segment}
}

func (p *reasoningParser) currentKind() contentKind {
	if p.inReasoning {
		return contentKindReasoning
	}
	return contentKindText
}

func (p *reasoningParser) appendSegment(segments *[]contentSegment, kind contentKind, text string) {
	if text == "" {
		return
	}
	if n := len(*segments); n > 0 && (*segments)[n-1].kind == kind {
		(*segments)[n-1].text += text
		return
	}
	*segments = append(*segments, contentSegment{kind: kind, text: text})
}
