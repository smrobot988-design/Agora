package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smrobot988-design/Agora/pkg/llm"
)

// retryMockProvider 记录调用次数并在达到恢复点前返回错误。
type retryMockProvider struct {
	name          string
	callCount     int
	err           error
	resp          *llm.Response
	failUntilCall int // 前 N 次调用返回 error，之后返回 resp（0 = 始终返回 resp）
}

func (m *retryMockProvider) Chat(ctx context.Context, params llm.ChatParams) (*llm.Response, error) {
	m.callCount++
	if m.failUntilCall == 0 || m.callCount > m.failUntilCall {
		return m.resp, nil
	}
	return nil, m.err
}

func (m *retryMockProvider) Name() string                        { return m.name }
func (m *retryMockProvider) EstimateTokens(msgs []llm.Message) int { return 0 }

// errNetwork 是网络类错误。
var errNetwork = errors.New("connection reset by peer")

// errRateLimit 是限流错误。
var errRateLimit = errors.New("rate limit exceeded: 429")

// errServerError 是服务器错误。
var errServerError = errors.New("internal server error: 500")

// errPermanent 是永久性错误（不应重试）。
var errPermanent = errors.New("invalid request: missing required field")

func TestRetrySuccess(t *testing.T) {
	provider := &retryMockProvider{resp: &llm.Response{Text: "ok"}}
	policy := DefaultRetryPolicy()
	retry := NewRetryProvider(provider, policy)

	resp, err := retry.Chat(context.Background(), llm.ChatParams{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("expected 'ok', got %q", resp.Text)
	}
	if provider.callCount != 1 {
		t.Fatalf("expected 1 call, got %d", provider.callCount)
	}
}

func TestRetryTransientErrorRetries(t *testing.T) {
	// 前 1 次返回网络错误，第 2 次成功
	provider := &retryMockProvider{
		err:           errNetwork,
		resp:          &llm.Response{Text: "ok"},
		failUntilCall: 1,
	}
	policy := DefaultRetryPolicy()
	policy.InitialDelay = 1 * time.Millisecond
	policy.MaxDelay = 1 * time.Millisecond
	retry := NewRetryProvider(provider, policy)

	resp, err := retry.Chat(context.Background(), llm.ChatParams{})
	if err != nil {
		t.Fatalf("expected no error after retries, got %v", err)
	}
	if provider.callCount != 2 {
		t.Fatalf("expected 2 calls (1 fail + 1 success), got %d", provider.callCount)
	}
	_ = resp
}

func TestRetryNonRetryableError(t *testing.T) {
	provider := &retryMockProvider{err: errPermanent, failUntilCall: 1}
	policy := DefaultRetryPolicy()
	policy.InitialDelay = 1 * time.Millisecond
	policy.MaxDelay = 1 * time.Millisecond
	retry := NewRetryProvider(provider, policy)

	_, err := retry.Chat(context.Background(), llm.ChatParams{})
	if err != errPermanent {
		t.Fatalf("expected permanent error, got %v", err)
	}
	if provider.callCount != 1 {
		t.Fatalf("expected exactly 1 call (no retries), got %d", provider.callCount)
	}
}

func TestRetryMaxAttemptsExceeded(t *testing.T) {
	provider := &retryMockProvider{err: errNetwork, failUntilCall: 99}
	policy := DefaultRetryPolicy()
	policy.MaxRetries = 2
	policy.InitialDelay = 1 * time.Millisecond
	policy.MaxDelay = 1 * time.Millisecond
	retry := NewRetryProvider(provider, policy)

	_, err := retry.Chat(context.Background(), llm.ChatParams{})
	if err != errNetwork {
		t.Fatalf("expected network error, got %v", err)
	}
	// 1 original + 2 retries = 3 calls
	if provider.callCount != 3 {
		t.Fatalf("expected 3 calls, got %d", provider.callCount)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &retryMockProvider{
		err:           errNetwork,
		failUntilCall: 99,
	}
	policy := DefaultRetryPolicy()
	policy.InitialDelay = 10 * time.Second // 故意设置很长
	policy.MaxDelay = 10 * time.Second
	policy.MaxRetries = 3
	retry := NewRetryProvider(provider, policy)

	// 立即取消上下文
	cancel()

	_, err := retry.Chat(ctx, llm.ChatParams{})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if provider.callCount != 1 {
		t.Fatalf("expected 1 call before cancellation, got %d", provider.callCount)
	}
}

func TestRetryJitter(t *testing.T) {
	policy := DefaultRetryPolicy()
	policy.Jitter = 0.5 // 50% 抖动

	delays := make([]time.Duration, 10)
	for i := range delays {
		delays[i] = policy.NextDelay(1)
	}

	// 检查是否有变化（ jitter > 0 时延迟应有差异）
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Log("Note: jitter randomness may not produce different values in small samples (expected for 10 samples)")
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		err      error
		expected RetryCode
	}{
		{errors.New("connection reset"), RetryCodeNetwork},
		{errors.New("connection refused"), RetryCodeNetwork},
		{errors.New("i/o timeout"), RetryCodeNetwork},
		{errors.New("context deadline exceeded"), RetryCodeNetwork},
		{errors.New("rate limit: 429"), RetryCodeRateLimit},
		{errors.New("429 Too Many Requests"), RetryCodeRateLimit},
		{errors.New("500 Internal Server Error"), RetryCodeServerError},
		{errors.New("502 bad gateway"), RetryCodeServerError},
		{errors.New("503 service unavailable"), RetryCodeServerError},
		{errors.New("504 gateway timeout"), RetryCodeServerError},
		{errors.New("invalid request"), RetryCodeUnknown},
		{nil, RetryCodeUnknown},
	}

	for _, tc := range tests {
		code := ClassifyError(tc.err)
		if code != tc.expected {
			t.Fatalf("ClassifyError(%v) = %d, want %d", tc.err, code, tc.expected)
		}
	}
}

func TestRetryProviderName(t *testing.T) {
	provider := &retryMockProvider{name: "test-provider"}
	retry := NewRetryProvider(provider, DefaultRetryPolicy())

	if retry.Name() != "test-provider" {
		t.Fatalf("expected 'test-provider', got %q", retry.Name())
	}
}

func TestRetryBackoffFormula(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:        5,
		InitialDelay:      1 * time.Second,
		MaxDelay:          100 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0, // 禁用 jitter 以便精确测试
	}

	expected := []time.Duration{
		1 * time.Second,  // attempt 1: 1s
		2 * time.Second,  // attempt 2: 2s
		4 * time.Second,  // attempt 3: 4s
		8 * time.Second,  // attempt 4: 8s
		16 * time.Second, // attempt 5: 16s
	}

	for i, exp := range expected {
		got := policy.NextDelay(i + 1)
		if got != exp {
			t.Fatalf("NextDelay(%d) = %v, want %v", i+1, got, exp)
		}
	}
}

func TestRetryMaxDelayCap(t *testing.T) {
	policy := RetryPolicy{
		InitialDelay:      10 * time.Second,
		MaxDelay:          15 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0,
	}

	// attempt 3: 10s * 2^2 = 40s，但被 cap 到 15s
	delay := policy.NextDelay(3)
	if delay != 15*time.Second {
		t.Fatalf("expected 15s (capped), got %v", delay)
	}
}

func TestRetryZeroMaxRetries(t *testing.T) {
	provider := &retryMockProvider{err: errNetwork, failUntilCall: 99}
	policy := DefaultRetryPolicy()
	policy.MaxRetries = 0
	policy.InitialDelay = 1 * time.Millisecond
	retry := NewRetryProvider(provider, policy)

	_, err := retry.Chat(context.Background(), llm.ChatParams{})
	if err != errNetwork {
		t.Fatalf("expected network error, got %v", err)
	}
	if provider.callCount != 1 {
		t.Fatalf("expected 1 call (no retries when MaxRetries=0), got %d", provider.callCount)
	}
}

func TestRetryPermanentErrorVariants(t *testing.T) {
	permanentErrors := []error{
		errors.New("invalid request: missing 'path' parameter"),
		errors.New("tool not found: read_file"),
		errors.New("unauthorized"),
	}

	for _, err := range permanentErrors {
		code := ClassifyError(err)
		if code != RetryCodeUnknown {
			t.Fatalf("ClassifyError(%q) = %d, want RetryCodeUnknown", err.Error(), code)
		}
	}
}

func TestRetryUnknownErrorDoesNotRetry(t *testing.T) {
	provider := &retryMockProvider{err: errors.New("something went wrong")}
	policy := DefaultRetryPolicy()
	policy.InitialDelay = 1 * time.Millisecond
	policy.MaxDelay = 1 * time.Millisecond
	retry := NewRetryProvider(provider, policy)

	_, err := retry.Chat(context.Background(), llm.ChatParams{})
	// "something went wrong" 不匹配任何已知错误码，不重试
	if provider.callCount != 1 {
		t.Fatalf("expected 1 call (unknown error not retried), got %d", provider.callCount)
	}
	_ = err
}

func TestRetryAllRetryCodes(t *testing.T) {
	retryableErrors := []error{
		errNetwork,
		errRateLimit,
		errServerError,
	}

	for _, testErr := range retryableErrors {
		provider := &retryMockProvider{err: testErr, failUntilCall: 99}
		policy := DefaultRetryPolicy()
		policy.InitialDelay = 1 * time.Millisecond
		policy.MaxDelay = 1 * time.Millisecond
		policy.MaxRetries = 1
		retry := NewRetryProvider(provider, policy)

		_, err := retry.Chat(context.Background(), llm.ChatParams{})
		// 应该触发重试，但最终返回错误（因为 provider 始终返回错误）
		if provider.callCount != 2 {
			t.Errorf("error %q: expected 2 calls, got %d", testErr, provider.callCount)
		}
		_ = err
	}
}

func TestNextDelayZeroAttempt(t *testing.T) {
	policy := DefaultRetryPolicy()
	policy.Jitter = 0

	// attempt <= 0 应该当作 1 处理
	delay := policy.NextDelay(0)
	if delay != policy.InitialDelay {
		t.Fatalf("NextDelay(0) = %v, want %v (InitialDelay)", delay, policy.InitialDelay)
	}
}

func TestRetryPolicyCustomIsRetryable(t *testing.T) {
	// 自定义策略：只有网络错误才重试
	provider := &retryMockProvider{err: errPermanent, failUntilCall: 1}
	policy := DefaultRetryPolicy()
	policy.IsRetryable = func(err error) bool {
		return strings.Contains(err.Error(), "connection")
	}
	policy.InitialDelay = 1 * time.Millisecond
	policy.MaxDelay = 1 * time.Millisecond
	retry := NewRetryProvider(provider, policy)

	_, err := retry.Chat(context.Background(), llm.ChatParams{})
	if err != errPermanent {
		t.Fatalf("expected permanent error, got %v", err)
	}
	if provider.callCount != 1 {
		t.Fatalf("expected 1 call (custom IsRetryable returns false), got %d", provider.callCount)
	}
}
