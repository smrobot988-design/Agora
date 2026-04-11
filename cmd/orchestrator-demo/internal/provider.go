package internal

import (
	"fmt"
	"os"

	_ "github.com/smrobot988-design/Agora/pkg/config"
	"github.com/smrobot988-design/Agora/pkg/llm"
)

// providerEnvKeys maps provider name to (env key for API key, env key for model if any).
var providerEnvKeys = map[string]struct{ keyEnv, modelEnv string }{
	"minimax":  {"MINIMAX_API_KEY", ""},
	"claude":   {"ANTHROPIC_API_KEY", ""},
	"deepseek": {"DEEPSEEK_API_KEY", ""},
	"doubao":   {"ARK_API_KEY", "ARK_MODEL"},
	"kimi":     {"MOONSHOT_API_KEY", ""},
	"glm":      {"GLM_API_KEY", ""},
	"gpt":      {"OPENAI_API_KEY", ""},
}

// NewProvider creates a provider by name from environment variables.
// Returns an error if the required API key is not set.
func NewProvider(name string) (llm.Provider, error) {
	info, ok := providerEnvKeys[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (use claude, minimax, deepseek, doubao, kimi, glm, gpt)", name)
	}

	apiKey := os.Getenv(info.keyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("%s not set (set env or .local.env)", info.keyEnv)
	}

	switch name {
	case "minimax":
		return llm.NewMiniMaxProvider(
			llm.MiniMaxWithAPIKey(apiKey),
			llm.MiniMaxWithModel("MiniMax-M2.7"),
			llm.MiniMaxWithMaxTokens(40960),
		), nil
	case "claude":
		opts := []llm.ClaudeOption{llm.WithAPIKey(apiKey)}
		if baseURL := os.Getenv("ANTHROPIC_BASE_URL"); baseURL != "" {
			opts = append(opts, llm.WithBaseURL(baseURL))
		}
		return llm.NewClaudeProvider(opts...), nil
	case "deepseek":
		return llm.NewDeepseekProvider(
			llm.DeepseekWithAPIKey(apiKey),
		), nil
	case "doubao":
		opts := []llm.DoubaoOption{llm.DoubaoWithAPIKey(apiKey)}
		if model := os.Getenv(info.modelEnv); model != "" {
			opts = append(opts, llm.DoubaoWithModel(model))
		}
		return llm.NewDoubaoProvider(opts...), nil
	case "kimi":
		return llm.NewKimiProvider(
			llm.KimiWithAPIKey(apiKey),
		), nil
	case "glm":
		return llm.NewGLMProvider(
			llm.GLMWithAPIKey(apiKey),
		), nil
	case "gpt":
		return llm.NewGPTProvider(
			llm.GPTWithAPIKey(apiKey),
		), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
}

// NewMiniMaxProvider creates a MiniMax provider from the MINIMAX_API_KEY environment variable.
// Returns an error if the API key is not set.
// Deprecated: Use NewProvider("minimax") instead.
func NewMiniMaxProvider() (*llm.MiniMaxProvider, error) {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("MINIMAX_API_KEY not set (set env or .local.env)")
	}
	return llm.NewMiniMaxProvider(
		llm.MiniMaxWithAPIKey(apiKey),
		llm.MiniMaxWithModel("MiniMax-M2.7"),
		llm.MiniMaxWithMaxTokens(40960),
	), nil
}
