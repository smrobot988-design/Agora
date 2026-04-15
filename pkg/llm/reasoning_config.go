package llm

// ThinkingMode controls whether a provider should keep its default behavior,
// explicitly enable internal reasoning, explicitly disable it, or let the
// provider auto-decide when such a mode exists.
type ThinkingMode string

const (
	ThinkingModeInherit ThinkingMode = ""
	ThinkingModeOn      ThinkingMode = "on"
	ThinkingModeOff     ThinkingMode = "off"
	ThinkingModeAuto    ThinkingMode = "auto"
)

// ThinkingEffort expresses the desired reasoning intensity in a provider-neutral
// way. Providers may map, clamp, or ignore unsupported levels.
type ThinkingEffort string

const (
	ThinkingEffortInherit ThinkingEffort = ""
	ThinkingEffortLow     ThinkingEffort = "low"
	ThinkingEffortMedium  ThinkingEffort = "medium"
	ThinkingEffortHigh    ThinkingEffort = "high"
	ThinkingEffortMax     ThinkingEffort = "max"
)

// ReasoningConfig configures provider-side thinking behavior and output parsing.
// A zero-value config preserves existing provider defaults.
type ReasoningConfig struct {
	Mode         ThinkingMode       `json:"mode,omitempty"`
	Effort       ThinkingEffort     `json:"effort,omitempty"`
	BudgetTokens int                `json:"budget_tokens,omitempty"`
	ParseMode    ReasoningParseMode `json:"parse_mode,omitempty"`
}

// HasOverrides reports whether the config asks Agora to override provider
// defaults in any way.
func (c ReasoningConfig) HasOverrides() bool {
	return c.Mode != ThinkingModeInherit ||
		c.Effort != ThinkingEffortInherit ||
		c.BudgetTokens > 0 ||
		c.ParseMode != ""
}

// Normalize returns a copy with unknown zero-values normalized to the
// backwards-compatible inherit/default behavior.
func (c ReasoningConfig) Normalize() ReasoningConfig {
	if c.Mode == "" {
		c.Mode = ThinkingModeInherit
	}
	if c.Effort == "" {
		c.Effort = ThinkingEffortInherit
	}
	return c
}

// AppliedReasoning reports how Agora actually handled reasoning for a request.
// It is safe to expose to business code and tracing.
type AppliedReasoning struct {
	Provider     string             `json:"provider,omitempty"`
	Source       string             `json:"source,omitempty"`
	Model        string             `json:"model,omitempty"`
	Mode         ThinkingMode       `json:"mode,omitempty"`
	Effort       ThinkingEffort     `json:"effort,omitempty"`
	BudgetTokens int                `json:"budget_tokens,omitempty"`
	ParseMode    ReasoningParseMode `json:"parse_mode,omitempty"`
	Notes        []string           `json:"notes,omitempty"`
}

func normalizeReasoningConfig(cfg *ReasoningConfig) ReasoningConfig {
	if cfg == nil {
		return ReasoningConfig{}.Normalize()
	}
	normalized := cfg.Normalize()
	return normalized
}

func defaultReasoningSource(cfg ReasoningConfig) string {
	if cfg.HasOverrides() {
		return "request"
	}
	return "provider_default"
}
