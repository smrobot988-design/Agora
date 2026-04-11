package llm

// ============================================================================
// GPT Provider (embeds OpenAICompatProvider)
// ============================================================================

// GPTProvider implements Provider using the OpenAI API.
type GPTProvider struct {
	*OpenAICompatProvider
}

type gptConfig struct {
	model     string
	maxTokens int
	apiKey    string
	baseURL   string
}

// GPTOption configures GPTProvider.
type GPTOption func(*gptConfig)

// GPTWithModel sets the GPT model (e.g. "gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "o1", "o3-mini").
func GPTWithModel(model string) GPTOption {
	return func(c *gptConfig) { c.model = model }
}

// GPTWithMaxTokens sets the maximum tokens for responses.
func GPTWithMaxTokens(n int) GPTOption {
	return func(c *gptConfig) { c.maxTokens = n }
}

// GPTWithAPIKey sets the API key directly instead of reading from environment.
func GPTWithAPIKey(key string) GPTOption {
	return func(c *gptConfig) { c.apiKey = key }
}

// GPTWithBaseURL sets a custom API base URL (e.g. for proxy/relay services).
func GPTWithBaseURL(url string) GPTOption {
	return func(c *gptConfig) { c.baseURL = url }
}

// NewGPTProvider creates an OpenAI GPT provider.
// By default reads OPENAI_API_KEY and OPENAI_BASE_URL from environment.
func NewGPTProvider(opts ...GPTOption) *GPTProvider {
	cfg := &gptConfig{
		model:     "gpt-5.4",
		maxTokens: 4096,
		baseURL:   getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &GPTProvider{
		OpenAICompatProvider: NewOpenAICompatProvider(OpenAICompatConfig{
			Name:      "gpt",
			BaseURL:   cfg.baseURL,
			APIKey:    cfg.apiKey,
			EnvKey:    "OPENAI_API_KEY",
			Model:     cfg.model,
			MaxTokens: cfg.maxTokens,
		}),
	}
}
