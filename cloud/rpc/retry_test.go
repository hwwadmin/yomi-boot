package rpc

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// TestIsRetryable 验证可重试 RPC 错误判定
func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"408", &HTTPStatusError{Code: http.StatusRequestTimeout}, true},
		{"429", &HTTPStatusError{Code: http.StatusTooManyRequests}, false},
		{"502", &HTTPStatusError{Code: http.StatusBadGateway}, true},
		{"503", &HTTPStatusError{Code: http.StatusServiceUnavailable}, true},
		{"504", &HTTPStatusError{Code: http.StatusGatewayTimeout}, true},
		{"400", &HTTPStatusError{Code: http.StatusBadRequest}, false},
		{"401", &HTTPStatusError{Code: http.StatusUnauthorized}, false},
		{"500", &HTTPStatusError{Code: http.StatusInternalServerError}, false},
		{"protocol", &ProtocolError{Reason: "bad body"}, false},
		{"request", &requestError{err: errors.New("bad url")}, false},
		{"discovery", &discoveryError{err: errors.New("consul down")}, true},
		{"network", wrapRetryable(errors.New("dial")), true},
		{"canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryable(tc.err); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// TestShouldRetry_DeadlineClassification 验证调用方还活着时单次超时可重试
func TestShouldRetry_DeadlineClassification(t *testing.T) {
	live := context.Background()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name        string
		ctx         context.Context
		err         error
		maxAttempts int
		want        bool
	}{
		{"attempt timeout", live, context.DeadlineExceeded, 3, true},
		{"url attempt timeout", live, &url.Error{Op: "Post", URL: "http://example.test", Err: context.DeadlineExceeded}, 3, true},
		{"retryable attempt timeout", live, wrapRetryable(context.DeadlineExceeded), 3, true},
		{"discovery timeout", live, &discoveryError{err: context.DeadlineExceeded}, 3, true},
		{"protocol deadline", live, &ProtocolError{Reason: "bad body", Err: context.DeadlineExceeded}, 3, false},
		{"local request deadline", live, &requestError{err: context.DeadlineExceeded}, 3, false},
		{"caller deadline", canceled, context.DeadlineExceeded, 3, false},
		{"attempts exhausted", live, context.DeadlineExceeded, 1, false},
		{"503", live, &HTTPStatusError{Code: http.StatusServiceUnavailable}, 3, true},
		{"500", live, &HTTPStatusError{Code: http.StatusInternalServerError}, 3, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRetry(tc.ctx, tc.err, 1, tc.maxAttempts); got != tc.want {
				t.Fatalf("shouldRetry() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestParseRetryAfter 验证 Retry-After 解析
func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("60"); got != time.Minute {
		t.Fatalf("60 秒: got %v want 1m", got)
	}
	if got := parseRetryAfter("bad"); got != 0 {
		t.Fatalf("非法值: got %v want 0", got)
	}
	if got := parseRetryAfter("-1"); got != 0 {
		t.Fatalf("负数: got %v want 0", got)
	}
	future := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 || got > 2*time.Minute {
		t.Fatalf("HTTP 日期: got %v want (0,2m]", got)
	}
}

// TestBackoffDuration 验证指数退避与 [d/2, d] 抖动范围
func TestBackoffDuration(t *testing.T) {
	base := 50 * time.Millisecond
	upper := 200 * time.Millisecond
	cases := []struct {
		attempt int
		lower   time.Duration
		upper   time.Duration
	}{
		{1, 25 * time.Millisecond, 50 * time.Millisecond},
		{2, 50 * time.Millisecond, 100 * time.Millisecond},
		{3, 100 * time.Millisecond, 200 * time.Millisecond},
		{10, 100 * time.Millisecond, 200 * time.Millisecond},
	}
	for _, tc := range cases {
		for i := 0; i < 50; i++ {
			d := backoffDuration(tc.attempt, base, upper)
			if d < tc.lower || d > tc.upper {
				t.Fatalf("attempt %d: got %v want [%v,%v]", tc.attempt, d, tc.lower, tc.upper)
			}
		}
	}
	if d := backoffDuration(1, 500*time.Millisecond, upper); d < 100*time.Millisecond || d > upper {
		t.Fatalf("base 超过上限时应夹到 [100ms,200ms]，实际 %v", d)
	}
}

// TestCanWait 验证剩余预算判断
func TestCanWait(t *testing.T) {
	if !canWait(context.Background(), time.Hour) {
		t.Fatal("无 deadline 应视为可以等待")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if canWait(ctx, 10*time.Second) {
		t.Fatal("剩余预算不足时应返回 false")
	}
	if !canWait(ctx, time.Millisecond) {
		t.Fatal("剩余预算充足时应返回 true")
	}
}
