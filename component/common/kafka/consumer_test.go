package kafka

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
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

// TestConsumeResultHelpers 验证三态辅助函数与零值
func TestConsumeResultHelpers(t *testing.T) {
	t.Parallel()

	if got := AckOK(); got != ConsumeOK {
		t.Fatalf("AckOK() = %s", got)
	}
	if got := Retry(); got != ConsumeRetry {
		t.Fatalf("Retry() = %s", got)
	}
	if got := AckDrop(); got != ConsumeDrop {
		t.Fatalf("AckDrop() = %s", got)
	}
	var zero ConsumeResult
	if zero != ConsumeRetry {
		t.Fatalf("零值 = %s，期望 retry", zero)
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

// TestBuildReaderConfigStartOffset 验证普通数字偏移返回错误，最早和最新可以显式指定
func TestBuildReaderConfigStartOffset(t *testing.T) {
	t.Parallel()

	runtimeCfg := consumerRuntime{minBytes: 1, maxBytes: 10e6, startOffset: kafkago.LastOffset}
	baseCfg := ConsumerConfig{Topics: []string{"orders"}, GroupID: "order-group"}

	t.Run("plain offset returns error", func(t *testing.T) {
		t.Parallel()
		cfg := baseCfg
		cfg.StartOffset = 1
		if _, err := buildReaderConfig(runtimeCfg, cfg); err == nil {
			t.Fatal("StartOffset=1 应返回错误，避免 kafka-go panic")
		}
	})

	t.Run("zero uses manager offset", func(t *testing.T) {
		t.Parallel()
		readerCfg, err := buildReaderConfig(runtimeCfg, baseCfg)
		if err != nil {
			t.Fatalf("未设置 StartOffset 应通过: %v", err)
		}
		if readerCfg.StartOffset != kafkago.LastOffset {
			t.Fatalf("StartOffset = %d, want %d", readerCfg.StartOffset, kafkago.LastOffset)
		}
	})

	for _, tc := range []struct {
		name   string
		offset int64
	}{
		{name: "explicit earliest", offset: kafkago.FirstOffset},
		{name: "explicit latest", offset: kafkago.LastOffset},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := baseCfg
			cfg.StartOffset = tc.offset
			readerCfg, err := buildReaderConfig(runtimeCfg, cfg)
			if err != nil {
				t.Fatalf("StartOffset=%d 应通过: %v", tc.offset, err)
			}
			if readerCfg.StartOffset != tc.offset {
				t.Fatalf("StartOffset = %d, want %d", readerCfg.StartOffset, tc.offset)
			}
		})
	}
}

// TestNewConsumerRuntimeStartOffset 验证管理器 start_offset 配置到 kafka-go 起始偏移量的映射
func TestNewConsumerRuntimeStartOffset(t *testing.T) {
	t.Parallel()

	conn := connection{
		brokers: []string{"127.0.0.1:9092"},
		dialer:  &kafkago.Dialer{Timeout: time.Second},
	}

	for _, tc := range []struct {
		name        string
		startOffset string
		want        int64
	}{
		{name: "latest maps to LastOffset", startOffset: "latest", want: kafkago.LastOffset},
		{name: "latest with spaces maps to LastOffset", startOffset: " latest ", want: kafkago.LastOffset},
		{name: "earliest maps to FirstOffset", startOffset: "earliest", want: kafkago.FirstOffset},
		{name: "empty maps to FirstOffset", startOffset: "", want: kafkago.FirstOffset},
		{name: "unknown value falls back to FirstOffset", startOffset: "middle", want: kafkago.FirstOffset},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := ConsumerManagerConfig{
				MinBytes:       1,
				MaxBytes:       10e6,
				MaxWaitSeconds: 1,
				StartOffset:    tc.startOffset,
			}
			runtimeCfg := newConsumerRuntime(cfg, conn)
			if runtimeCfg.startOffset != tc.want {
				t.Fatalf("startOffset = %d, want %d", runtimeCfg.startOffset, tc.want)
			}
			if len(runtimeCfg.brokers) != 1 || runtimeCfg.brokers[0] != "127.0.0.1:9092" {
				t.Fatalf("brokers = %v, want [127.0.0.1:9092]", runtimeCfg.brokers)
			}
			if runtimeCfg.minBytes != 1 || runtimeCfg.maxBytes != 10e6 {
				t.Fatalf("min/max bytes = %d/%d, want 1/10000000", runtimeCfg.minBytes, runtimeCfg.maxBytes)
			}
			if runtimeCfg.maxWait != time.Second {
				t.Fatalf("maxWait = %v, want 1s", runtimeCfg.maxWait)
			}
			if runtimeCfg.dialer == nil {
				t.Fatal("dialer 应原样透传")
			}
		})
	}
}

// scriptedReader 按预设结果提交偏移量，并记录提交发生的时刻
type scriptedReader struct {
	commitErr error
	onCommit  func()
}

func (r *scriptedReader) FetchMessage(context.Context) (kafkago.Message, error) {
	return kafkago.Message{}, errors.New("未使用")
}

func (r *scriptedReader) CommitMessages(context.Context, ...kafkago.Message) error {
	if r.onCommit != nil {
		r.onCommit()
	}
	return r.commitErr
}

func (r *scriptedReader) Close() error { return nil }

// TestDropLogFollowsCommit 验证丢弃日志在提交成功之后，提交失败时不报已丢弃
func TestDropLogFollowsCommit(t *testing.T) {
	msg := Message{Topic: "orders", Partition: 1, Offset: 7}
	kafkaMsg := kafkago.Message{Topic: msg.Topic, Partition: msg.Partition, Offset: msg.Offset}

	var order []string
	consumeLogObserver = func(kind string) {
		order = append(order, kind)
	}
	t.Cleanup(func() { consumeLogObserver = nil })

	t.Run("drop logs after commit", func(t *testing.T) {
		order = nil
		reader := &scriptedReader{onCommit: func() { order = append(order, "commit") }}
		c := &consumer{reader: reader, maxHandlerRetries: -1}
		err := c.processMessageWithRetry(context.Background(), func(context.Context, Message) ConsumeResult {
			return AckDrop()
		}, msg, kafkaMsg)
		if err != nil {
			t.Fatalf("提交成功应返回 nil: %v", err)
		}
		if got, want := joinOrder(order), "commit,drop"; got != want {
			t.Fatalf("顺序 = %s，期望 %s", got, want)
		}
	})

	t.Run("retry exhausted logs after commit", func(t *testing.T) {
		order = nil
		reader := &scriptedReader{onCommit: func() { order = append(order, "commit") }}
		c := &consumer{reader: reader, maxHandlerRetries: 0}
		err := c.processMessageWithRetry(context.Background(), func(context.Context, Message) ConsumeResult {
			return Retry()
		}, msg, kafkaMsg)
		if err != nil {
			t.Fatalf("提交成功应返回 nil: %v", err)
		}
		if got, want := joinOrder(order), "commit,drop-exceeded"; got != want {
			t.Fatalf("顺序 = %s，期望 %s", got, want)
		}
	})

	t.Run("unknown result does not commit", func(t *testing.T) {
		committed := false
		reader := &scriptedReader{onCommit: func() { committed = true }}
		c := &consumer{reader: reader, maxHandlerRetries: -1}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.processMessageWithRetry(ctx, func(context.Context, Message) ConsumeResult {
			return ConsumeResult(99)
		}, msg, kafkaMsg)
		if err == nil {
			t.Fatal("未知结果应按重试等待，上下文取消后返回错误")
		}
		if committed {
			t.Fatal("未知结果不应提交偏移量")
		}
	})

	t.Run("commit failure does not log dropped", func(t *testing.T) {
		order = nil
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reader := &scriptedReader{
			commitErr: context.Canceled,
			onCommit:  func() { order = append(order, "commit") },
		}
		c := &consumer{reader: reader, maxHandlerRetries: 0}
		err := c.processMessageWithRetry(ctx, func(context.Context, Message) ConsumeResult {
			return AckDrop()
		}, msg, kafkaMsg)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("提交失败应返回上下文错误: %v", err)
		}
		if got, want := joinOrder(order), "commit,commit-failed"; got != want {
			t.Fatalf("顺序 = %s，期望 %s", got, want)
		}
	})
}

func joinOrder(order []string) string {
	return strings.Join(order, ",")
}

func TestConsumerHelpers_Edges(t *testing.T) {
	if ConsumeOK.String() != "ok" {
		t.Fatalf("成功结果名称 = %s", ConsumeOK.String())
	}
	if got := nextRetryDelay(consumerRetryMaxDelay); got <= 0 {
		t.Fatalf("退避应仍为正数: %s", got)
	}
	if firstPositive(0, -1) != 0 {
		t.Fatal("没有正数时应返回 0")
	}
	if firstDuration(0, -time.Second) != 0 {
		t.Fatal("没有正时长时应返回 0")
	}
	c := &consumer{closed: true}
	if err := c.close(); err != nil {
		t.Fatal(err)
	}
	prev := consumeLogObserver
	t.Cleanup(func() { consumeLogObserver = prev })
	var seen string
	consumeLogObserver = func(kind string) { seen = kind }
	notifyConsumeLog("drop")
	if seen != "drop" {
		t.Fatalf("观察者收到 %q", seen)
	}
}
