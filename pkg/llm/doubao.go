package llm

// ============================================================================
// Doubao Provider (embeds OpenAICompatProvider)
// ============================================================================

// DoubaoProvider implements Provider using the Doubao (豆包) OpenAI-compatible API
// via Volcano Engine (火山引擎 ARK).
type DoubaoProvider struct {
	*OpenAICompatProvider
}

type doubaoConfig struct {
	model     string
	maxTokens int
	apiKey    string
	baseURL   string
}

// DoubaoOption configures DoubaoProvider.
type DoubaoOption func(*doubaoConfig)

// DoubaoWithModel sets the Doubao endpoint ID.
// Note: Doubao uses endpoint IDs (e.g. "ep-20240901xxxxx") as the model parameter,
// not model names. You must create an endpoint on the Volcano Engine ARK console first.
func DoubaoWithModel(model string) DoubaoOption {
	return func(c *doubaoConfig) { c.model = model }
}

// DoubaoWithMaxTokens sets the maximum tokens for responses.
func DoubaoWithMaxTokens(n int) DoubaoOption {
	return func(c *doubaoConfig) { c.maxTokens = n }
}

// DoubaoWithAPIKey sets the API key directly instead of reading from environment.
func DoubaoWithAPIKey(key string) DoubaoOption {
	return func(c *doubaoConfig) { c.apiKey = key }
}

// DoubaoWithBaseURL sets a custom API base URL.
func DoubaoWithBaseURL(url string) DoubaoOption {
	return func(c *doubaoConfig) { c.baseURL = url }
}

// NewDoubaoProvider creates a Doubao provider.
// By default reads ARK_API_KEY from environment.
// You must set the endpoint ID via DoubaoWithModel (e.g. "ep-20240901xxxxx").
func NewDoubaoProvider(opts ...DoubaoOption) *DoubaoProvider {
	cfg := &doubaoConfig{
		model:     "", // user must specify endpoint ID
		maxTokens: 4096,
		baseURL:   "https://ark.cn-beijing.volces.com/api/v3",
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &DoubaoProvider{
		OpenAICompatProvider: NewOpenAICompatProvider(OpenAICompatConfig{
			Name:      "doubao",
			BaseURL:   cfg.baseURL,
			APIKey:    cfg.apiKey,
			EnvKey:    "ARK_API_KEY",
			Model:     cfg.model,
			MaxTokens: cfg.maxTokens,
		}),
	}
}
