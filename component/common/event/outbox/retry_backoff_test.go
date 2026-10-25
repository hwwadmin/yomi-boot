// Package outbox 的死信重试退避测试
//
// @author zwei
package outbox

import (
	"testing"
	"time"
)

// TestDlqRetryDelayBase 验证指数退避基础时长与上限
func TestDlqRetryDelayBase(t *testing.T) {
	cases := []struct {
		retryCount int
		want       time.Duration
	}{
		{retryCount: 0, want: 0},
		{retryCount: -3, want: 0},
		{retryCount: 1, want: 1 * time.Second},
		{retryCount: 2, want: 2 * time.Second},
		{retryCount: 3, want: 4 * time.Second},
		{retryCount: 4, want: 8 * time.Second},
		{retryCount: 5, want: 16 * time.Second},
		{retryCount: 6, want: 32 * time.Second},
		{retryCount: 7, want: dlqRetryMaxDelay},
		{retryCount: 8, want: dlqRetryMaxDelay},
		{retryCount: 100, want: dlqRetryMaxDelay},
	}
	for _, tc := range cases {
		if got := dlqRetryDelayBase(tc.retryCount); got != tc.want {
			t.Fatalf("retryCount=%d 期望 %s，实际 %s", tc.retryCount, tc.want, got)
		}
	}
}

// TestDlqRetryDelay 验证含抖动的退避时长落在基础时长的 80%~120% 区间
func TestDlqRetryDelay(t *testing.T) {
	if got := dlqRetryDelay(0); got != 0 {
		t.Fatalf("retryCount<=0 期望 0，实际 %s", got)
	}
	if got := dlqRetryDelay(-1); got != 0 {
		t.Fatalf("retryCount<=0 期望 0，实际 %s", got)
	}

	cases := []struct {
		retryCount int
		base       time.Duration
	}{
		{retryCount: 1, base: time.Second},
		{retryCount: 3, base: 4 * time.Second},
		{retryCount: 8, base: dlqRetryMaxDelay},
	}
	for _, tc := range cases {
		lower := tc.base * dlqRetryJitterMin / dlqRetryJitterDenom
		upper := tc.base * dlqRetryJitterMax / dlqRetryJitterDenom
		for i := 0; i < 50; i++ {
			got := dlqRetryDelay(tc.retryCount)
			if got < lower || got > upper {
				t.Fatalf("retryCount=%d 退避时长 %s 超出 [%s, %s]", tc.retryCount, got, lower, upper)
			}
		}
	}
}

// TestJitterDelay 验证抖动区间边界
func TestJitterDelay(t *testing.T) {
	if got := jitterDelay(0); got != 0 {
		t.Fatalf("非正时长应原样返回，实际 %s", got)
	}
	if got := jitterDelay(-time.Second); got != -time.Second {
		t.Fatalf("负时长应原样返回，实际 %s", got)
	}

	delay := 10 * time.Second
	lower := delay * dlqRetryJitterMin / dlqRetryJitterDenom
	upper := delay * dlqRetryJitterMax / dlqRetryJitterDenom
	for i := 0; i < 100; i++ {
		got := jitterDelay(delay)
		if got < lower || got > upper {
			t.Fatalf("抖动时长 %s 超出 [%s, %s]", got, lower, upper)
		}
	}
}
