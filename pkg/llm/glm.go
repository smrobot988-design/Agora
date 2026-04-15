package llm

// ============================================================================
// GLM Provider (embeds OpenAICompatProvider)
// ============================================================================

// GLMProvider implements Provider using the Zhipu AI (智谱) OpenAI-compatible API.
type GLMProvider struct {
	*OpenAICompatProvider
}

type glmConfig struct {
	model         string
	maxTokens     int
	apiKey        string
	baseURL       string
	reasoningMode ReasoningMode
}

// GLMOption configures GLMProvider.
type GLMOption func(*glmConfig)

// GLMWithModel sets the GLM model (e.g. "glm-4", "glm-4-flash", "glm-4-plus").
func GLMWithModel(model string) GLMOption {
	return func(c *glmConfig) { c.model = model }
}

// GLMWithMaxTokens sets the maximum tokens for responses.
func GLMWithMaxTokens(n int) GLMOption {
	return func(c *glmConfig) { c.maxTokens = n }
}

// GLMWithAPIKey sets the API key directly instead of reading from environment.
func GLMWithAPIKey(key string) GLMOption {
	return func(c *glmConfig) { c.apiKey = key }
}

// GLMWithBaseURL sets a custom API base URL.
func GLMWithBaseURL(url string) GLMOption {
	return func(c *glmConfig) { c.baseURL = url }
}

// GLMWithReasoningMode sets how GLM output is classified into
// reasoning and final text content.
func GLMWithReasoningMode(mode ReasoningMode) GLMOption {
	return func(c *glmConfig) { c.reasoningMode = mode }
}

// NewGLMProvider creates a GLM (Zhipu AI) provider.
// By default reads GLM_API_KEY from environment.
func NewGLMProvider(opts ...GLMOption) *GLMProvider {
	cfg := &glmConfig{
		model:         "glm-4",
		maxTokens:     4096,
		baseURL:       "https://open.bigmodel.cn/api/paas/v4",
		reasoningMode: ReasoningModeNone,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &GLMProvider{
		OpenAICompatProvider: NewOpenAICompatProvider(OpenAICompatConfig{
			Name:          "glm",
			BaseURL:       cfg.baseURL,
			APIKey:        cfg.apiKey,
			EnvKey:        "GLM_API_KEY",
			Model:         cfg.model,
			MaxTokens:     cfg.maxTokens,
			ReasoningMode: cfg.reasoningMode,
		}),
	}
}
