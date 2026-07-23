package rpc

import (
	"context"
	"net/http"
	"testing"
	"time"
	"yomi-boot/errors"
)

// TestIsRetryableRPCError 验证可重试 RPC 错误判定
func TestIsRetryableRPCError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"biz", errors.NewBiz("INVALID_INPUT", "bad"), false},
		{"retryable wrap", wrapRetryable(errors.New("dial")), true},
		{"502", &httpStatusError{code: http.StatusBadGateway, detail: "bad"}, true},
		{"503", &httpStatusError{code: http.StatusServiceUnavailable, detail: "ua"}, true},
		{"400", &httpStatusError{code: http.StatusBadRequest, detail: "bad"}, false},
		{"canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableRPCError(tc.err); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
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
