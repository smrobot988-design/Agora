package llm

// ============================================================================
// Kimi Provider (embeds OpenAICompatProvider)
// ============================================================================

// KimiProvider implements Provider using the Moonshot AI (Kimi) OpenAI-compatible API.
type KimiProvider struct {
	*OpenAICompatProvider
}

type kimiConfig struct {
	model         string
	maxTokens     int
	apiKey        string
	baseURL       string
	reasoningMode ReasoningMode
}

// KimiOption configures KimiProvider.
type KimiOption func(*kimiConfig)

// KimiWithModel sets the Kimi model (e.g. "moonshot-v1-8k", "moonshot-v1-32k", "moonshot-v1-128k").
func KimiWithModel(model string) KimiOption {
	return func(c *kimiConfig) { c.model = model }
}

// KimiWithMaxTokens sets the maximum tokens for responses.
func KimiWithMaxTokens(n int) KimiOption {
	return func(c *kimiConfig) { c.maxTokens = n }
}

// KimiWithAPIKey sets the API key directly instead of reading from environment.
func KimiWithAPIKey(key string) KimiOption {
	return func(c *kimiConfig) { c.apiKey = key }
}

// KimiWithBaseURL sets a custom API base URL.
func KimiWithBaseURL(url string) KimiOption {
	return func(c *kimiConfig) { c.baseURL = url }
}

// KimiWithReasoningMode sets how Kimi output is classified into
// reasoning and final text content.
func KimiWithReasoningMode(mode ReasoningMode) KimiOption {
	return func(c *kimiConfig) { c.reasoningMode = mode }
}

// NewKimiProvider creates a Kimi (Moonshot AI) provider.
// By default reads MOONSHOT_API_KEY from environment.
func NewKimiProvider(opts ...KimiOption) *KimiProvider {
	cfg := &kimiConfig{
		model:         "moonshot-v1-8k",
		maxTokens:     4096,
		baseURL:       "https://api.moonshot.cn/v1",
		reasoningMode: ReasoningModeNone,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &KimiProvider{
		OpenAICompatProvider: NewOpenAICompatProvider(OpenAICompatConfig{
			Name:          "kimi",
			BaseURL:       cfg.baseURL,
			APIKey:        cfg.apiKey,
			EnvKey:        "MOONSHOT_API_KEY",
			Model:         cfg.model,
			MaxTokens:     cfg.maxTokens,
			ReasoningMode: cfg.reasoningMode,
		}),
	}
}
