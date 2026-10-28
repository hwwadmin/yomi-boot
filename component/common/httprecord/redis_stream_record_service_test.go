package httprecord

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
)

// TestRedisStreamHttpRecordService_Handle 验证异步写入 Redis Stream
func TestRedisStreamHttpRecordService_Handle(t *testing.T) {
	t.Parallel()

	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	components, err := redisstream.NewComponents(redisstream.Config{}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	const stream = "httprecord.test"
	svc := NewRedisStreamHttpRecordService(components.Publisher, stream)
	svc.Handle(context.Background(), &HttpRecord{
		TraceID: "t-1",
		Method:  "GET",
		Path:    "/ping",
	})

	deadline := time.Now().Add(2 * time.Second)
	var payload []byte
	for time.Now().Before(deadline) {
		msgs, err := client.XRange(context.Background(), stream, "-", "+").Result()
		if err != nil {
			t.Fatalf("XRange: %v", err)
		}
		if len(msgs) > 0 {
			raw, _ := msgs[0].Values[redisstream.PayloadField].(string)
			payload = []byte(raw)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(payload) == 0 {
		t.Fatal("timeout waiting for redis stream message")
	}

	var wire wireRecord
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if wire.TraceID != "t-1" || wire.Path != "/ping" {
		t.Fatalf("wire = %+v, want traceId=t-1 path=/ping", wire)
	}
}

// TestStartRedisStreamRecordConsumer 验证消费端点反序列化并委托 handler
func TestStartRedisStreamRecordConsumer(t *testing.T) {
	t.Parallel()

	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	components, err := redisstream.NewComponents(redisstream.Config{}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	const stream = "httprecord.consume"
	got := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = StartRedisStreamRecordConsumer(ctx, components.Consumer, RedisStreamConsumerID, RedisStreamRecordConsumerConfig{
		Stream:   stream,
		Group:    "httprecord-test",
		Consumer: "c1",
	}, func(_ context.Context, record *HttpRecord) RecordHandleResult {
		got <- record.TraceID
		return HandleOK()
	})
	if err != nil {
		t.Fatalf("StartRedisStreamRecordConsumer: %v", err)
	}

	svc := NewRedisStreamHttpRecordService(components.Publisher, stream)
	svc.Handle(context.Background(), &HttpRecord{TraceID: "consume-1", Method: "POST", Path: "/api"})

	select {
	case traceID := <-got:
		if traceID != "consume-1" {
			t.Fatalf("traceId = %q, want consume-1", traceID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for redis stream consumer")
	}
}

// TestMapRecordHandlerResultToRedisStream 验证 RecordHandleResult 到 Redis Stream HandleResult 的映射
func TestMapRecordHandlerResultToRedisStream(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   RecordHandleResult
		want redisstream.HandleResult
	}{
		{name: "ok", in: HandleOK(), want: redisstream.HandleAck},
		{name: "retry", in: HandleRetry(), want: redisstream.HandleRetry},
		{name: "drop", in: HandleDrop(), want: redisstream.HandleDead},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := mapRecordHandlerResultToRedisStream(tc.in); got != tc.want {
				t.Fatalf("mapRecordHandlerResultToRedisStream() = %v, want %v", got, tc.want)
			}
		})
	}
}

// blockingStreamPublisher 阻塞到投递上下文结束的 Redis Stream 发布器桩
type blockingStreamPublisher struct{}

// Add 空实现
func (blockingStreamPublisher) Add(context.Context, string, map[string]string) (string, error) {
	return "", nil
}

// AddBytes 空实现
func (blockingStreamPublisher) AddBytes(context.Context, string, []byte) (string, error) {
	return "", nil
}

// AddJSON 空实现
func (blockingStreamPublisher) AddJSON(context.Context, string, any) (string, error) { return "", nil }

// AddAsync 空实现
func (blockingStreamPublisher) AddAsync(context.Context, string, map[string]string) error {
	return nil
}

// AddBytesAsync 空实现
func (blockingStreamPublisher) AddBytesAsync(context.Context, string, []byte) error { return nil }

// AddJSONAsync 阻塞到投递上下文结束
func (blockingStreamPublisher) AddJSONAsync(ctx context.Context, _ string, _ any) error {
	<-ctx.Done()
	return ctx.Err()
}

// Close 空实现
func (blockingStreamPublisher) Close() error { return nil }

// TestRedisStreamRecordHandleBudget 验证发布阻塞时投递只等独立短预算
func TestRedisStreamRecordHandleBudget(t *testing.T) {
	svc := NewRedisStreamHttpRecordService(blockingStreamPublisher{}, "s")

	start := time.Now()
	svc.Handle(context.Background(), &HttpRecord{TraceID: "budget", Method: "GET", Path: "/x"})
	elapsed := time.Since(start)

	if elapsed < recordPublishBudget/2 {
		t.Fatalf("投递应等待预算，实际耗时 %v", elapsed)
	}
	if elapsed > recordPublishBudget+2*time.Second {
		t.Fatalf("投递阻塞超出预算: %v", elapsed)
	}
}
