package core

import (
	"context"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/smrobot988-design/Agora/pkg/llm"
)

// RetryCode 表示可重试错误的分类。
type RetryCode int

const (
	RetryCodeUnknown       RetryCode = iota // 未知错误，不重试
	RetryCodeNetwork                        // 网络问题（连接断开/拒绝/超时）
	RetryCodeRateLimit                      // 限流（429）
	RetryCodeServerError                    // 服务器错误（500/502/503/504）
)

// ClassifyError 将 error 分类为 RetryCode。
// 对于 LLM provider 返回的错误，提取 HTTP 状态码或错误信息关键词进行分类。
func ClassifyError(err error) RetryCode {
	if err == nil {
		return RetryCodeUnknown
	}
	msg := strings.ToLower(err.Error())
	// HTTP 状态码检查优先，避免 "504 gateway timeout" 被 generic "timeout" 误捕获
	switch {
	case strings.Contains(msg, "500"),
		strings.Contains(msg, "502"),
		strings.Contains(msg, "503"),
		strings.Contains(msg, "504"):
		return RetryCodeServerError
	case strings.Contains(msg, "429"):
		return RetryCodeRateLimit
	case strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "context deadline exceeded"):
		// 注意：generic "timeout" 放最后，避免误匹配 "504 gateway timeout" 等
		return RetryCodeNetwork
	case strings.Contains(msg, "timeout"):
		return RetryCodeNetwork
	default:
		return RetryCodeUnknown
	}
}

// DefaultIsRetryable 根据 RetryCode 判断是否应重试。
func DefaultIsRetryable(err error) bool {
	code := ClassifyError(err)
	return code == RetryCodeNetwork || code == RetryCodeRateLimit || code == RetryCodeServerError
}

// RetryPolicy 定义何时重试及退避策略。
type RetryPolicy struct {
	MaxRetries        int           // 最大重试次数（0 = 不重试）
	InitialDelay      time.Duration // 首次重试前等待时间
	MaxDelay          time.Duration // 最大等待时间上限
	BackoffMultiplier float64       // 退避倍数。例如 2.0：第1次重试=1s，第2次=2s，第3次=4s
	Jitter            float64       // 随机抖动系数（0.0-1.0）。0.1 表示 ±10% 的随机延迟，防止惊群效应
	IsRetryable       func(err error) bool
}

// DefaultRetryPolicy 返回一个开箱即用的重试策略。
// - 最多重试 3 次
// - 首次延迟 1s，之后指数增长，上限 30s
// - ±10% 随机抖动
// - 网络错误、限流、服务器错误可重试
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.1,
		IsRetryable:       DefaultIsRetryable,
	}
}

// NextDelay 计算第 attempt 次重试的延迟（attempt 从 1 开始）。
// 公式：min(InitialDelay × BackoffMultiplier^(attempt-1) × (1 ± Jitter), MaxDelay)
func (p RetryPolicy) NextDelay(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	delay := float64(p.InitialDelay) * math.Pow(p.BackoffMultiplier, float64(attempt-1))
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	if p.Jitter > 0 {
		// (rand*2-1) 生成 [-1, 1] 范围的随机数
		delay += (rand.Float64()*2 - 1) * delay * p.Jitter
	}
	return time.Duration(delay)
}

// RetryProvider 装饰 llm.Provider，添加重试逻辑。
type RetryProvider struct {
	inner  llm.Provider
	policy RetryPolicy
}

// NewRetryProvider 使用指定策略包装一个 Provider。
func NewRetryProvider(inner llm.Provider, policy RetryPolicy) *RetryProvider {
	return &RetryProvider{inner: inner, policy: policy}
}

// Chat 调用底层 Provider，遇到可重试错误时按策略重试。
func (p *RetryProvider) Chat(ctx context.Context, params llm.ChatParams) (*llm.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= p.policy.MaxRetries; attempt++ {
		resp, err := p.inner.Chat(ctx, params)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// 不可重试的错误或已耗尽重试次数，直接返回
		if !p.policy.IsRetryable(err) || attempt >= p.policy.MaxRetries {
			return nil, err
		}

		// 计算延迟并等待
		delay := p.policy.NextDelay(attempt + 1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			// 继续下一次重试
		}
	}
	return nil, lastErr
}

func (p *RetryProvider) Name() string { return p.inner.Name() }

func (p *RetryProvider) EstimateTokens(msgs []llm.Message) int {
	return p.inner.EstimateTokens(msgs)
}
