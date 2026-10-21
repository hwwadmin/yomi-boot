package rpc

import (
	"context"
	"net/http"
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

// TestBackoffDuration 验证退避时长计算
func TestBackoffDuration(t *testing.T) {
	base := 50 * time.Millisecond
	max := 200 * time.Millisecond
	if d := backoffDuration(1, base, max); d != base {
		t.Fatalf("attempt1: got %v want %v", d, base)
	}
	if d := backoffDuration(2, base, max); d != 100*time.Millisecond {
		t.Fatalf("attempt2: got %v want 100ms", d)
	}
	if d := backoffDuration(3, base, max); d != max {
		t.Fatalf("attempt3: got %v want %v", d, max)
	}
}
