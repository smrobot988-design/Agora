package llm

// ============================================================================
// Deepseek Provider (embeds OpenAICompatProvider)
// ============================================================================

// DeepseekProvider implements Provider using the Deepseek OpenAI-compatible API.
type DeepseekProvider struct {
	*OpenAICompatProvider
}

type deepseekConfig struct {
	model     string
	maxTokens int
	apiKey    string
	baseURL   string
}

// DeepseekOption configures DeepseekProvider.
type DeepseekOption func(*deepseekConfig)

// DeepseekWithModel sets the Deepseek model (e.g. "deepseek-chat", "deepseek-reasoner").
func DeepseekWithModel(model string) DeepseekOption {
	return func(c *deepseekConfig) { c.model = model }
}

// DeepseekWithMaxTokens sets the maximum tokens for responses.
func DeepseekWithMaxTokens(n int) DeepseekOption {
	return func(c *deepseekConfig) { c.maxTokens = n }
}

// DeepseekWithAPIKey sets the API key directly instead of reading from environment.
func DeepseekWithAPIKey(key string) DeepseekOption {
	return func(c *deepseekConfig) { c.apiKey = key }
}

// DeepseekWithBaseURL sets a custom API base URL.
func DeepseekWithBaseURL(url string) DeepseekOption {
	return func(c *deepseekConfig) { c.baseURL = url }
}

// NewDeepseekProvider creates a Deepseek provider.
// By default reads DEEPSEEK_API_KEY from environment.
func NewDeepseekProvider(opts ...DeepseekOption) *DeepseekProvider {
	cfg := &deepseekConfig{
		model:     "deepseek-chat",
		maxTokens: 4096,
		baseURL:   "https://api.deepseek.com/v1",
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &DeepseekProvider{
		OpenAICompatProvider: NewOpenAICompatProvider(OpenAICompatConfig{
			Name:      "deepseek",
			BaseURL:   cfg.baseURL,
			APIKey:    cfg.apiKey,
			EnvKey:    "DEEPSEEK_API_KEY",
			Model:     cfg.model,
			MaxTokens: cfg.maxTokens,
		}),
	}
}
