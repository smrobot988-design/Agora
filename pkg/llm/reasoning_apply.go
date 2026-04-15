package llm

import "github.com/anthropics/anthropic-sdk-go"

const claudeDefaultThinkingBudget int64 = 1024

type openAIReasoningResolution struct {
	model           string
	reasoningEffort string
	parseMode       ReasoningParseMode
	applied         *AppliedReasoning
}

func resolveOpenAICompatReasoning(providerName, baseModel string, defaultParseMode ReasoningParseMode, cfg *ReasoningConfig) openAIReasoningResolution {
	requested := normalizeReasoningConfig(cfg)
	parseMode := defaultParseMode
	if requested.ParseMode != "" {
		parseMode = requested.ParseMode
	}

	applied := &AppliedReasoning{
		Provider:  providerName,
		Source:    defaultReasoningSource(requested),
		Model:     baseModel,
		Mode:      requested.Mode,
		ParseMode: parseMode,
	}

	model := baseModel
	notes := make([]string, 0, 2)

	switch providerName {
	case "gpt":
		switch requested.Mode {
		case ThinkingModeOn:
			if requested.ParseMode == "" {
				parseMode = ReasoningParseModeNative
			}
		case ThinkingModeOff:
			if requested.ParseMode == "" {
				parseMode = ReasoningParseModeNone
			}
			notes = append(notes, "explicit thinking disable is not mapped for OpenAI chat completions; model behavior remains provider-defined")
		case ThinkingModeAuto:
			notes = append(notes, "thinking auto is not explicitly configurable for OpenAI chat completions; model behavior remains provider-defined")
		}

		switch requested.Effort {
		case ThinkingEffortLow, ThinkingEffortMedium, ThinkingEffortHigh:
			if requested.ParseMode == "" {
				parseMode = ReasoningParseModeNative
			}
			applied.Effort = requested.Effort
		case ThinkingEffortMax:
			if requested.ParseMode == "" {
				parseMode = ReasoningParseModeNative
			}
			applied.Effort = ThinkingEffortHigh
			notes = append(notes, "OpenAI chat completions do not support effort=max; downgraded to high")
		}

		if requested.BudgetTokens > 0 {
			notes = append(notes, "budget_tokens is not supported for OpenAI chat completions and was ignored")
		}

	case "deepseek":
		switch requested.Mode {
		case ThinkingModeOn:
			model = "deepseek-reasoner"
			if requested.ParseMode == "" {
				parseMode = ReasoningParseModeNative
			}
		case ThinkingModeOff:
			model = "deepseek-chat"
			if requested.ParseMode == "" {
				parseMode = ReasoningParseModeNone
			}
		case ThinkingModeAuto:
			notes = append(notes, "thinking auto is not defined for DeepSeek; kept the current model")
		}

		if requested.Effort != ThinkingEffortInherit {
			model = "deepseek-reasoner"
			if requested.ParseMode == "" {
				parseMode = ReasoningParseModeNative
			}
			applied.Mode = ThinkingModeOn
			notes = append(notes, "DeepSeek does not expose provider-neutral effort levels here; switched to deepseek-reasoner")
		}
		if requested.BudgetTokens > 0 {
			notes = append(notes, "budget_tokens is not supported for DeepSeek and was ignored")
		}

	case "kimi", "glm", "doubao":
		switch requested.Mode {
		case ThinkingModeOn:
			if requested.ParseMode == "" {
				parseMode = ReasoningParseModeNative
			}
			notes = append(notes, "explicit thinking enable is not wired in the current OpenAI-compatible transport; kept provider defaults")
		case ThinkingModeOff:
			if requested.ParseMode == "" {
				parseMode = ReasoningParseModeNone
			}
			notes = append(notes, "explicit thinking disable is not wired in the current OpenAI-compatible transport; kept provider defaults")
		case ThinkingModeAuto:
			notes = append(notes, "thinking auto is not wired in the current OpenAI-compatible transport; kept provider defaults")
		}
		if requested.Effort != ThinkingEffortInherit {
			notes = append(notes, "thinking effort is not wired in the current OpenAI-compatible transport and was ignored")
		}
		if requested.BudgetTokens > 0 {
			notes = append(notes, "budget_tokens is not supported in the current OpenAI-compatible transport and was ignored")
		}

	case "minimax":
		switch requested.Mode {
		case ThinkingModeOn:
			notes = append(notes, "MiniMax explicit thinking control is not wired here; kept provider defaults")
		case ThinkingModeOff:
			notes = append(notes, "MiniMax explicit thinking disable is not wired here; kept provider defaults")
		case ThinkingModeAuto:
			notes = append(notes, "MiniMax thinking auto is not wired here; kept provider defaults")
		}
		if requested.Effort != ThinkingEffortInherit {
			notes = append(notes, "MiniMax thinking effort is not wired here and was ignored")
		}
		if requested.BudgetTokens > 0 {
			notes = append(notes, "MiniMax budget_tokens is not supported here and was ignored")
		}
	}

	applied.Model = model
	applied.ParseMode = parseMode
	if len(notes) > 0 {
		applied.Notes = notes
	}

	return openAIReasoningResolution{
		model:           model,
		reasoningEffort: string(applied.Effort),
		parseMode:       parseMode,
		applied:         applied,
	}
}

type claudeReasoningResolution struct {
	thinking    anthropic.ThinkingConfigParamUnion
	useThinking bool
	output      anthropic.OutputConfigParam
	useOutput   bool
	parseMode   ReasoningParseMode
	applied     *AppliedReasoning
}

func resolveClaudeReasoning(model anthropic.Model, cfg *ReasoningConfig) claudeReasoningResolution {
	requested := normalizeReasoningConfig(cfg)
	parseMode := ReasoningParseModeNone
	if requested.ParseMode != "" {
		parseMode = requested.ParseMode
	}

	applied := &AppliedReasoning{
		Provider:  "claude",
		Source:    defaultReasoningSource(requested),
		Model:     string(model),
		Mode:      requested.Mode,
		ParseMode: parseMode,
	}
	notes := make([]string, 0, 2)

	var thinking anthropic.ThinkingConfigParamUnion
	useThinking := false

	switch requested.Mode {
	case ThinkingModeOn:
		budget := int64(requested.BudgetTokens)
		if budget == 0 {
			budget = claudeDefaultThinkingBudget
			notes = append(notes, "Claude thinking budget defaulted to 1024 tokens")
		}
		thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
		useThinking = true
		applied.BudgetTokens = int(budget)
		if requested.ParseMode == "" {
			parseMode = ReasoningParseModeNative
		}
	case ThinkingModeOff:
		disabled := anthropic.NewThinkingConfigDisabledParam()
		thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &disabled}
		useThinking = true
		if requested.ParseMode == "" {
			parseMode = ReasoningParseModeNone
		}
	case ThinkingModeAuto:
		adaptive := anthropic.ThinkingConfigAdaptiveParam{}
		thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive}
		useThinking = true
		if requested.ParseMode == "" {
			parseMode = ReasoningParseModeNative
		}
	}

	if requested.BudgetTokens > 0 && requested.Mode == ThinkingModeInherit {
		thinking = anthropic.ThinkingConfigParamOfEnabled(int64(requested.BudgetTokens))
		useThinking = true
		applied.Mode = ThinkingModeOn
		applied.BudgetTokens = requested.BudgetTokens
		if requested.ParseMode == "" {
			parseMode = ReasoningParseModeNative
		}
	}

	var output anthropic.OutputConfigParam
	useOutput := false
	switch requested.Effort {
	case ThinkingEffortLow:
		output.Effort = anthropic.OutputConfigEffortLow
		applied.Effort = ThinkingEffortLow
		useOutput = true
	case ThinkingEffortMedium:
		output.Effort = anthropic.OutputConfigEffortMedium
		applied.Effort = ThinkingEffortMedium
		useOutput = true
	case ThinkingEffortHigh:
		output.Effort = anthropic.OutputConfigEffortHigh
		applied.Effort = ThinkingEffortHigh
		useOutput = true
	case ThinkingEffortMax:
		output.Effort = anthropic.OutputConfigEffortMax
		applied.Effort = ThinkingEffortMax
		useOutput = true
	}

	if useOutput && !useThinking && requested.Mode == ThinkingModeInherit {
		notes = append(notes, "Claude effort was set without explicitly changing thinking mode; provider default thinking availability remains in effect")
		if requested.ParseMode == "" {
			parseMode = ReasoningParseModeNative
		}
	}

	applied.ParseMode = parseMode
	if len(notes) > 0 {
		applied.Notes = notes
	}

	return claudeReasoningResolution{
		thinking:    thinking,
		useThinking: useThinking,
		output:      output,
		useOutput:   useOutput,
		parseMode:   parseMode,
		applied:     applied,
	}
}
