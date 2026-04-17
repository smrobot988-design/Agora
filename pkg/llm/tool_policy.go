package llm

// ToolChoiceMode controls whether the provider may choose tools freely,
// must call a tool, must call a specific tool, or must not call tools.
type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = ""
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceSpecific ToolChoiceMode = "tool"
	ToolChoiceNone     ToolChoiceMode = "none"
)

// ToolCallPolicy configures provider-agnostic tool-call constraints and
// framework-side recovery behavior.
type ToolCallPolicy struct {
	Choice            ToolChoiceMode `json:"choice,omitempty"`
	ToolName          string         `json:"tool_name,omitempty"`
	DisableParallel   bool           `json:"disable_parallel,omitempty"`
	StrictSchema      bool           `json:"strict_schema,omitempty"`
	MaxRepairAttempts int            `json:"max_repair_attempts,omitempty"`
}

// Normalize returns a copy with backwards-compatible defaults applied.
func (p ToolCallPolicy) Normalize() ToolCallPolicy {
	if p.Choice == "" {
		p.Choice = ToolChoiceAuto
	}
	if p.Choice == ToolChoiceSpecific {
		p.DisableParallel = true
		if p.ToolName == "" {
			p.Choice = ToolChoiceRequired
		}
	}
	if p.MaxRepairAttempts < 0 {
		p.MaxRepairAttempts = 0
	}
	if p.MaxRepairAttempts == 0 {
		p.MaxRepairAttempts = 1
	}
	return p
}

// RequiresToolCall reports whether the request requires at least one tool call.
func (p ToolCallPolicy) RequiresToolCall() bool {
	switch p.Normalize().Choice {
	case ToolChoiceRequired, ToolChoiceSpecific:
		return true
	default:
		return false
	}
}

func normalizeToolCallPolicy(policy *ToolCallPolicy) ToolCallPolicy {
	if policy == nil {
		return ToolCallPolicy{}.Normalize()
	}
	normalized := policy.Normalize()
	return normalized
}
