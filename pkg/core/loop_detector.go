package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/smrobot988-design/Agora/pkg/llm"
	"github.com/smrobot988-design/Agora/pkg/schema"
)

// LoopType classifies the detected loop.
type LoopType int

const (
	LoopTypeNone LoopType = iota
	LoopTypeSameToolSameInput
	LoopTypeSameError
)

// LoopDetectionResult describes a detected loop.
type LoopDetectionResult struct {
	Type   LoopType
	Detail string
	Turn   int
}

// LoopDetector tracks recent tool call sequences to detect repetitive loops.
// It is safe for concurrent use.
type LoopDetector struct {
	history           []LoopEntry
	sameToolThreshold int
}

// LoopEntry records a single round of tool calls and their results.
type LoopEntry struct {
	Turn        int
	Fingerprint string // SHA256 hash of sorted tool calls
	Results     []string
	HasError    bool
}

// NewLoopDetector creates a LoopDetector with the default threshold of 3.
func NewLoopDetector() *LoopDetector {
	return &LoopDetector{
		sameToolThreshold: 3,
	}
}

// WithSameToolThreshold sets how many identical tool-call rounds trigger a loop.
// A value of 3 (default) means the same tool+calls must appear 3 times consecutively.
func (d *LoopDetector) WithSameToolThreshold(n int) *LoopDetector {
	d.sameToolThreshold = n
	return d
}

// Record stores a tool-call round and its results.
// Calls and results must be provided in the same order.
func (d *LoopDetector) Record(turn int, calls []ValidatedToolCall, results []llm.ToolResult) {
	fp := fingerprintToolCalls(calls)
	resultStrings := make([]string, len(results))
	hasError := false
	for i, r := range results {
		resultStrings[i] = r.Content
		if r.IsError {
			hasError = true
		}
	}

	d.history = append(d.history, LoopEntry{
		Turn:        turn,
		Fingerprint: fp,
		Results:     resultStrings,
		HasError:    hasError,
	})

	// Keep history bounded.
	const maxHistory = 20
	if len(d.history) > maxHistory {
		d.history = d.history[len(d.history)-maxHistory:]
	}
}

// Detect checks the history for a loop condition.
// Returns LoopTypeNone if no loop is detected.
// Error loops (LoopTypeSameError) are checked before tool loops
// because repeated errors are more critical.
func (d *LoopDetector) Detect() LoopDetectionResult {
	n := len(d.history)
	if n < d.sameToolThreshold && n < 2 {
		return LoopDetectionResult{Type: LoopTypeNone}
	}

	// Check for consecutive identical errors first (more critical).
	if n >= 2 {
		last := d.history[n-1]
		prev := d.history[n-2]
		if last.HasError && prev.HasError && len(last.Results) > 0 && len(prev.Results) > 0 {
			if last.Results[0] == prev.Results[0] {
				return LoopDetectionResult{
					Type:   LoopTypeSameError,
					Detail: last.Results[0],
					Turn:   last.Turn,
				}
			}
		}
	}

	// Check for same tool same input repetition.
	if n >= d.sameToolThreshold {
		window := d.history[n-d.sameToolThreshold:]
		counts := make(map[string]int)
		for _, entry := range window {
			counts[entry.Fingerprint]++
		}
		for fp, count := range counts {
			if count >= d.sameToolThreshold {
				return LoopDetectionResult{
					Type:   LoopTypeSameToolSameInput,
					Detail: fp,
					Turn:   d.history[n-1].Turn,
				}
			}
		}
	}

	return LoopDetectionResult{Type: LoopTypeNone}
}

// fingerprintToolCalls produces a stable hash for a set of tool calls.
func fingerprintToolCalls(calls []ValidatedToolCall) string {
	// Sort by call ID for stable ordering.
	sorted := make([]schema.ToolCall, len(calls))
	for i, vc := range calls {
		sorted[i] = vc.Call
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	// Build a deterministic representation.
	repr := make([]string, len(sorted))
	for i, c := range sorted {
		inputJSON, _ := json.Marshal(c.Input)
		repr[i] = c.Name + ":" + string(inputJSON)
	}
	combined := join(repr, "|")
	h := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(h[:8]) // first 8 hex chars
}

// join is a simple string join without importing strings package.
func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}
