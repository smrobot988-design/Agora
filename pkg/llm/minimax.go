package llm

// ============================================================================
// MiniMax Provider (embeds OpenAICompatProvider)
// ============================================================================

// MiniMaxProvider implements Provider using the MiniMax OpenAI-compatible API.
type MiniMaxProvider struct {
	*OpenAICompatProvider
}

// miniMaxConfig collects option values before construction.
type miniMaxConfig struct {
	model         string
	maxTokens     int
	apiKey        string
	baseURL       string
	reasoningMode ReasoningMode
}

// MiniMaxOption configures MiniMaxProvider.
type MiniMaxOption func(*miniMaxConfig)

// MiniMaxWithModel sets the MiniMax model to use.
func MiniMaxWithModel(model string) MiniMaxOption {
	return func(c *miniMaxConfig) { c.model = model }
}

// MiniMaxWithMaxTokens sets the maximum tokens for responses.
func MiniMaxWithMaxTokens(n int) MiniMaxOption {
	return func(c *miniMaxConfig) { c.maxTokens = n }
}

// MiniMaxWithAPIKey sets the API key directly instead of reading from environment.
func MiniMaxWithAPIKey(key string) MiniMaxOption {
	return func(c *miniMaxConfig) { c.apiKey = key }
}

// MiniMaxWithBaseURL sets a custom API base URL (e.g. for proxy/relay services).
func MiniMaxWithBaseURL(url string) MiniMaxOption {
	return func(c *miniMaxConfig) { c.baseURL = url }
}

// MiniMaxWithReasoningMode sets how MiniMax output is classified into
// reasoning and final text content.
func MiniMaxWithReasoningMode(mode ReasoningMode) MiniMaxOption {
	return func(c *miniMaxConfig) { c.reasoningMode = mode }
}

// NewMiniMaxProvider creates a MiniMax provider.
// By default reads MINIMAX_API_KEY from environment.
func NewMiniMaxProvider(opts ...MiniMaxOption) *MiniMaxProvider {
	cfg := &miniMaxConfig{
		model:         "MiniMax-M2.7",
		maxTokens:     4096,
		baseURL:       "https://api.minimax.chat/v1",
		reasoningMode: ReasoningModeThinkTag,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &MiniMaxProvider{
		OpenAICompatProvider: NewOpenAICompatProvider(OpenAICompatConfig{
			Name:          "minimax",
			BaseURL:       cfg.baseURL,
			APIKey:        cfg.apiKey,
			EnvKey:        "MINIMAX_API_KEY",
			Model:         cfg.model,
			MaxTokens:     cfg.maxTokens,
			ReasoningMode: cfg.reasoningMode,
		}),
	}
}
