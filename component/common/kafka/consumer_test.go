package kafka

import (
	"context"
	"testing"
	"time"
)

// TestInvokeConsumeHandler 验证 handler panic 被捕获并按 Retry 处理
func TestInvokeConsumeHandler(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	msg := Message{Topic: "orders", Partition: 1, Offset: 42}

	t.Run("normal handler", func(t *testing.T) {
		t.Parallel()
		got := invokeConsumeHandler(ctx, func(_ context.Context, _ Message) ConsumeResult {
			return AckOK()
		}, msg)
		if got != AckOK() {
			t.Fatalf("got %+v, want %+v", got, AckOK())
		}
	})

	t.Run("handler panic returns retry", func(t *testing.T) {
		t.Parallel()
		got := invokeConsumeHandler(ctx, func(_ context.Context, _ Message) ConsumeResult {
			panic("handler boom")
		}, msg)
		if got != Retry() {
			t.Fatalf("got %+v, want %+v", got, Retry())
		}
	})
}

// TestNormalizeConsumeResult 验证消费结果规范化逻辑，包括合法组合与非法组合的纠偏处理
func TestNormalizeConsumeResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name  string
		input ConsumeResult
		want  ConsumeResult
	}{
		{
			name:  "ok with ack",
			input: AckOK(),
			want:  AckOK(),
		},
		{
			name:  "retry without ack",
			input: Retry(),
			want:  Retry(),
		},
		{
			name:  "drop with ack",
			input: AckDrop(),
			want:  AckDrop(),
		},
		{
			name: "retry with ack coerced",
			input: ConsumeResult{
				Status: ConsumeStatusRetry,
				Ack:    true,
			},
			want: Retry(),
		},
		{
			name: "ok without ack coerced",
			input: ConsumeResult{
				Status: ConsumeStatusOK,
				Ack:    false,
			},
			want: Retry(),
		},
		{
			name: "drop without ack coerced",
			input: ConsumeResult{
				Status: ConsumeStatusDrop,
				Ack:    false,
			},
			want: Retry(),
		},
		{
			name: "unknown status with ack",
			input: ConsumeResult{
				Status: ConsumeStatus(99),
				Ack:    true,
			},
			want: AckOK(),
		},
		{
			name: "unknown status without ack",
			input: ConsumeResult{
				Status: ConsumeStatus(99),
				Ack:    false,
			},
			want: Retry(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeConsumeResult(ctx, tt.input)
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestConsumeResultHelpers 验证 AckOK、Retry、AckDrop 辅助函数返回正确的状态与 Ack 组合
func TestConsumeResultHelpers(t *testing.T) {
	t.Parallel()

	if got := AckOK(); got.Status != ConsumeStatusOK || !got.Ack {
		t.Fatalf("AckOK() = %+v", got)
	}
	if got := Retry(); got.Status != ConsumeStatusRetry || got.Ack {
		t.Fatalf("Retry() = %+v", got)
	}
	if got := AckDrop(); got.Status != ConsumeStatusDrop || !got.Ack {
		t.Fatalf("AckDrop() = %+v", got)
	}
}

// TestWaitForRetryRespectsContext 验证上下文超时或取消时 waitForRetry 立即返回错误
func TestWaitForRetryRespectsContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := waitForRetry(ctx, time.Second)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestJitterDelayWithinRange 验证抖动延迟落在 [80%, 120%] 区间内
func TestJitterDelayWithinRange(t *testing.T) {
	t.Parallel()

	base := 10 * time.Second
	min := base * consumerRetryJitterMin / consumerRetryJitterDenom
	max := base * consumerRetryJitterMax / consumerRetryJitterDenom

	for range 100 {
		got := jitterDelay(base)
		if got < min || got > max {
			t.Fatalf("jitterDelay(%v) = %v, want within [%v, %v]", base, got, min, max)
		}
	}
}

// TestJitterDelayEdgeCases 验证 jitterDelay 在零值和负值等边界输入下的行为
func TestJitterDelayEdgeCases(t *testing.T) {
	t.Parallel()

	if got := jitterDelay(0); got != 0 {
		t.Fatalf("jitterDelay(0) = %v, want 0", got)
	}
	if got := jitterDelay(-time.Second); got != -time.Second {
		t.Fatalf("jitterDelay(-1s) = %v, want -1s", got)
	}
}

// TestEffectiveMaxHandlerRetries 验证 handler 最大重试次数的默认值与显式值解析
func TestEffectiveMaxHandlerRetries(t *testing.T) {
	t.Parallel()

	t.Run("default unlimited", func(t *testing.T) {
		t.Parallel()
		got, err := effectiveMaxHandlerRetries(ConsumerConfig{})
		if err != nil {
			t.Fatalf("effectiveMaxHandlerRetries: %v", err)
		}
		if got != -1 {
			t.Fatalf("got %d, want -1", got)
		}
	})

	t.Run("explicit values", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			value int
			want  int
		}{
			{value: -1, want: -1},
			{value: 0, want: 0},
			{value: 3, want: 3},
		} {
			tc := tc
			t.Run("", func(t *testing.T) {
				t.Parallel()
				value := tc.value
				got, err := effectiveMaxHandlerRetries(ConsumerConfig{MaxHandlerRetries: &value})
				if err != nil {
					t.Fatalf("effectiveMaxHandlerRetries: %v", err)
				}
				if got != tc.want {
					t.Fatalf("got %d, want %d", got, tc.want)
				}
			})
		}
	})

	t.Run("invalid value", func(t *testing.T) {
		t.Parallel()
		value := -2
		_, err := effectiveMaxHandlerRetries(ConsumerConfig{MaxHandlerRetries: &value})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestBuildReaderConfigMinMaxBytes 验证最小拉取字节数大于最大时返回错误，而不是让 kafka-go panic
func TestBuildReaderConfigMinMaxBytes(t *testing.T) {
	t.Parallel()

	baseCfg := ConsumerConfig{Topics: []string{"orders"}, GroupID: "order-group"}

	t.Run("manager defaults invalid", func(t *testing.T) {
		t.Parallel()
		runtimeCfg := consumerRuntime{minBytes: 10485760, maxBytes: 1048576}
		if _, err := buildReaderConfig(runtimeCfg, baseCfg); err == nil {
			t.Fatal("管理器默认 min_bytes > max_bytes 应校验失败")
		}
	})

	t.Run("consumer override conflicts with manager default", func(t *testing.T) {
		t.Parallel()
		runtimeCfg := consumerRuntime{minBytes: 100, maxBytes: 10e6}
		cfg := baseCfg
		cfg.MaxBytes = 50
		if _, err := buildReaderConfig(runtimeCfg, cfg); err == nil {
			t.Fatal("单次 max_bytes 小于管理器 min_bytes 应校验失败")
		}
	})

	t.Run("valid combination", func(t *testing.T) {
		t.Parallel()
		runtimeCfg := consumerRuntime{minBytes: 1, maxBytes: 10e6}
		readerCfg, err := buildReaderConfig(runtimeCfg, baseCfg)
		if err != nil {
			t.Fatalf("合法组合应通过校验: %v", err)
		}
		if readerCfg.MinBytes != 1 || readerCfg.MaxBytes != 10e6 {
			t.Fatalf("MinBytes/MaxBytes = %d/%d, want 1/10000000", readerCfg.MinBytes, readerCfg.MaxBytes)
		}
	})
}
