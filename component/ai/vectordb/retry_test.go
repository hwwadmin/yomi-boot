// 限流退避重试单测。
//
// @author zwei

package vectordb

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestRateLimitDetection 覆盖限流错误识别
func TestRateLimitDetection(t *testing.T) {
	t.Parallel()
	if isRateLimitErr(nil) {
		t.Fatal("nil should not be rate limit")
	}
	yes := []string{"rate limit exceeded", "RATELIMIT", "proxy RateLimiter", "server rate limit reached"}
	for _, msg := range yes {
		if !isRateLimitErr(errors.New(msg)) {
			t.Fatalf("%q should be rate limit", msg)
		}
	}
	for _, msg := range []string{"not found", "timeout", "quota exceeded"} {
		if isRateLimitErr(errors.New(msg)) {
			t.Fatalf("%q should not be rate limit", msg)
		}
	}
}

// TestRateLimitBackoffSequence 验证指数退避与 12s 上限
func TestRateLimitBackoffSequence(t *testing.T) {
	t.Parallel()
	want := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		12 * time.Second,
		12 * time.Second,
	}
	for i, expected := range want {
		if got := rateLimitBackoff(i); got != expected {
			t.Fatalf("attempt %d: got %v, want %v", i, got, expected)
		}
	}
}

// TestRetryOnRateLimitValue 覆盖成功、非限流、限流后恢复与上下文取消
func TestRetryOnRateLimitValue(t *testing.T) {
	t.Parallel()
	t.Run("success first", func(t *testing.T) {
		t.Parallel()
		calls := 0
		got, err := retryOnRateLimitVal(context.Background(), func() (int, error) {
			calls++
			return 7, nil
		})
		if err != nil || got != 7 || calls != 1 {
			t.Fatalf("got=%d err=%v calls=%d", got, err, calls)
		}
	})

	t.Run("non rate limit", func(t *testing.T) {
		t.Parallel()
		want := errors.New("boom")
		calls := 0
		_, err := retryOnRateLimitVal(context.Background(), func() (int, error) {
			calls++
			return 0, want
		})
		if !errors.Is(err, want) || calls != 1 {
			t.Fatalf("err=%v calls=%d", err, calls)
		}
	})

	t.Run("retry then success", func(t *testing.T) {
		t.Parallel()
		calls := 0
		got, err := retryOnRateLimitVal(context.Background(), func() (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("rate limit")
			}
			return "ok", nil
		})
		if err != nil || got != "ok" || calls != 2 {
			t.Fatalf("got=%q err=%v calls=%d", got, err, calls)
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		_, err := retryOnRateLimitVal(ctx, func() (int, error) {
			calls++
			return 0, errors.New("rate limit")
		})
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("err=%v calls=%d", err, calls)
		}
	})
}

// TestRetryOnRateLimitVoid 覆盖无返回值包装
func TestRetryOnRateLimitVoid(t *testing.T) {
	t.Parallel()
	calls := 0
	err := retryOnRateLimit(context.Background(), func() error {
		calls++
		if calls == 1 {
			return errors.New("ratelimit")
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
