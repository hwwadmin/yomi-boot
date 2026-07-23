// Package kafkatest 提供 Kafka 内存测试 broker 的集成测试
//
// @author zwei
package kafkatest

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/kafka"

	kafkago "github.com/segmentio/kafka-go"
)

// TestMockBroker_PublishAndConsume 验证通过 mock broker 发布消息后消费者可正常拉取并提交偏移量
func TestMockBroker_PublishAndConsume(t *testing.T) {
	const (
		topic   = "orders"
		groupID = "orders-consumer"
	)

	broker := NewMockBroker(t, SeedTopics(1, topic))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := broker.Publish(ctx, topic, kafkago.Message{
		Key:   []byte("k1"),
		Value: []byte("v1"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	reader := broker.NewReader(kafkago.ReaderConfig{
		Topic:   topic,
		GroupID: groupID,
	})
	defer func() { _ = reader.Close() }()

	msg, err := reader.FetchMessage(ctx)
	if err != nil {
		t.Fatalf("FetchMessage: %v", err)
	}
	if string(msg.Key) != "k1" || string(msg.Value) != "v1" {
		t.Fatalf("unexpected message: key=%q value=%q", msg.Key, msg.Value)
	}
	if err := reader.CommitMessages(ctx, msg); err != nil {
		t.Fatalf("CommitMessages: %v", err)
	}
}

// TestMockBroker_NewComponents 验证 NewComponents 创建的发布者与消费者可正常收发消息
func TestMockBroker_NewComponents(t *testing.T) {
	const topic = "events"

	components, broker := NewComponents(t, topic)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := components.Publisher.Publish(ctx, kafka.PublishRequest{
		Topic: topic,
		Value: []byte("payload"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	handlerCalled := make(chan struct{}, 1)
	err := components.Consumer.Start(ctx, "test-consumer", broker.ConsumerConfig([]string{topic}, "events-group"), func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		if string(msg.Value) != "payload" {
			t.Errorf("unexpected value: %q", msg.Value)
		}
		select {
		case handlerCalled <- struct{}{}:
		default:
		}
		return kafka.AckOK()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-handlerCalled:
	case <-ctx.Done():
		t.Fatal("handler was not called before timeout")
	}

	if err := components.Consumer.Stop("test-consumer"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestMockBroker_ConsumerRetriesSameMessage 验证 handler 返回 Retry 时在同一条消息上重试直至成功
func TestMockBroker_ConsumerRetriesSameMessage(t *testing.T) {
	const topic = "retry-events"

	components, broker := NewComponents(t, topic)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := components.Publisher.Publish(ctx, kafka.PublishRequest{
		Topic: topic,
		Value: []byte("retry-me"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	var attempts atomic.Int32
	var doneOnce sync.Once
	handlerDone := make(chan struct{})

	err := components.Consumer.Start(ctx, "retry-consumer", broker.ConsumerConfig([]string{topic}, "retry-group"), func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		if string(msg.Value) != "retry-me" {
			t.Errorf("unexpected value: %q", msg.Value)
		}
		if attempts.Add(1) < 3 {
			return kafka.Retry()
		}
		doneOnce.Do(func() { close(handlerDone) })
		return kafka.AckOK()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-handlerDone:
	case <-ctx.Done():
		t.Fatal("handler did not succeed after retries")
	}

	if got := attempts.Load(); got != 3 {
		t.Fatalf("handler attempts = %d, want 3", got)
	}

	if err := components.Consumer.Stop("retry-consumer"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestMockBroker_ConsumerAckDrop 验证 handler 返回 AckDrop 时跳过当前消息并提交偏移量
func TestMockBroker_ConsumerAckDrop(t *testing.T) {
	const topic = "drop-events"

	components, broker := NewComponents(t, topic)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := components.Publisher.Publish(ctx, kafka.PublishRequest{
		Topic: topic,
		Value: []byte("drop-me"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	handlerCalled := make(chan struct{}, 1)
	err := components.Consumer.Start(ctx, "drop-consumer", broker.ConsumerConfig([]string{topic}, "drop-group"), func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		if string(msg.Value) != "drop-me" {
			t.Errorf("unexpected value: %q", msg.Value)
		}
		select {
		case handlerCalled <- struct{}{}:
		default:
		}
		return kafka.AckDrop()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-handlerCalled:
	case <-ctx.Done():
		t.Fatal("handler was not called before timeout")
	}

	if err := components.Consumer.Stop("drop-consumer"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// maxHandlerRetriesPtr 返回 MaxHandlerRetries 指针，便于在测试中修改消费配置
func maxHandlerRetriesPtr(v int) *int {
	return &v
}

// TestMockBroker_ConsumerMaxRetriesZero 验证 MaxHandlerRetries=0 时 handler 返回 Retry 仅执行一次即丢弃
func TestMockBroker_ConsumerMaxRetriesZero(t *testing.T) {
	const topic = "max-retries-zero"

	components, broker := NewComponents(t, topic)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := components.Publisher.Publish(ctx, kafka.PublishRequest{
		Topic: topic,
		Value: []byte("poison"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	var attempts atomic.Int32
	handlerDone := make(chan struct{}, 1)
	cfg := broker.ConsumerConfig([]string{topic}, "max-zero-group")
	cfg.MaxHandlerRetries = maxHandlerRetriesPtr(0)

	err := components.Consumer.Start(ctx, "max-zero-consumer", cfg, func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		attempts.Add(1)
		select {
		case handlerDone <- struct{}{}:
		default:
		}
		return kafka.Retry()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-handlerDone:
	case <-ctx.Done():
		t.Fatal("handler was not called before timeout")
	}

	if got := attempts.Load(); got != 1 {
		t.Fatalf("handler attempts = %d, want 1", got)
	}

	if err := components.Consumer.Stop("max-zero-consumer"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestMockBroker_ConsumerMaxRetriesExceeded 验证 handler 重试超限后自动丢弃当前消息并继续消费下一条
func TestMockBroker_ConsumerMaxRetriesExceeded(t *testing.T) {
	const topic = "max-retries-exceeded"

	components, broker := NewComponents(t, topic)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := components.Publisher.Publish(ctx, kafka.PublishRequest{
		Topic: topic,
		Value: []byte("poison"),
	}); err != nil {
		t.Fatalf("Publish poison: %v", err)
	}
	if err := components.Publisher.Publish(ctx, kafka.PublishRequest{
		Topic: topic,
		Value: []byte("healthy"),
	}); err != nil {
		t.Fatalf("Publish healthy: %v", err)
	}

	var poisonAttempts atomic.Int32
	healthyDone := make(chan struct{}, 1)
	cfg := broker.ConsumerConfig([]string{topic}, "max-exceeded-group")
	cfg.MaxHandlerRetries = maxHandlerRetriesPtr(2)

	err := components.Consumer.Start(ctx, "max-exceeded-consumer", cfg, func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		switch string(msg.Value) {
		case "poison":
			poisonAttempts.Add(1)
			return kafka.Retry()
		case "healthy":
			select {
			case healthyDone <- struct{}{}:
			default:
			}
			return kafka.AckOK()
		default:
			t.Errorf("unexpected value: %q", msg.Value)
			return kafka.AckDrop()
		}
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-healthyDone:
	case <-ctx.Done():
		t.Fatal("healthy message was not consumed before timeout")
	}

	if got := poisonAttempts.Load(); got != 3 {
		t.Fatalf("poison handler attempts = %d, want 3", got)
	}

	if err := components.Consumer.Stop("max-exceeded-consumer"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestMockBroker_ConsumerHandlerPanic 验证 handler panic 按可重试处理，超限后丢弃并继续消费下一条
func TestMockBroker_ConsumerHandlerPanic(t *testing.T) {
	const topic = "panic-events"

	components, broker := NewComponents(t, topic)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := components.Publisher.Publish(ctx, kafka.PublishRequest{
		Topic: topic,
		Value: []byte("poison"),
	}); err != nil {
		t.Fatalf("Publish poison: %v", err)
	}
	if err := components.Publisher.Publish(ctx, kafka.PublishRequest{
		Topic: topic,
		Value: []byte("healthy"),
	}); err != nil {
		t.Fatalf("Publish healthy: %v", err)
	}

	var poisonAttempts atomic.Int32
	healthyDone := make(chan struct{}, 1)
	cfg := broker.ConsumerConfig([]string{topic}, "panic-group")
	cfg.MaxHandlerRetries = maxHandlerRetriesPtr(1)

	err := components.Consumer.Start(ctx, "panic-consumer", cfg, func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		switch string(msg.Value) {
		case "poison":
			poisonAttempts.Add(1)
			panic("handler boom")
		case "healthy":
			select {
			case healthyDone <- struct{}{}:
			default:
			}
			return kafka.AckOK()
		default:
			t.Errorf("unexpected value: %q", msg.Value)
			return kafka.AckDrop()
		}
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-healthyDone:
	case <-ctx.Done():
		t.Fatal("healthy message was not consumed before timeout")
	}

	if got := poisonAttempts.Load(); got != 2 {
		t.Fatalf("poison handler attempts = %d, want 2", got)
	}

	if err := components.Consumer.Stop("panic-consumer"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestMockBroker_ConsumerInvalidMaxHandlerRetries 验证 MaxHandlerRetries 非法值时 Start 返回错误
func TestMockBroker_ConsumerInvalidMaxHandlerRetries(t *testing.T) {
	components, broker := NewComponents(t, "invalid-retries")
	ctx := context.Background()

	cfg := broker.ConsumerConfig([]string{"invalid-retries"}, "invalid-group")
	cfg.MaxHandlerRetries = maxHandlerRetriesPtr(-2)

	err := components.Consumer.Start(ctx, "invalid-consumer", cfg, func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		return kafka.AckOK()
	})
	if err == nil {
		t.Fatal("expected Start to fail for invalid MaxHandlerRetries")
	}
}

// TestMockBroker_ConsumerStopIdempotent 验证 Stop 对已停止或上下文已取消的消费者幂等返回 nil
func TestMockBroker_ConsumerStopIdempotent(t *testing.T) {
	const topic = "stop-idempotent"

	components, broker := NewComponents(t, topic)
	ctx, cancel := context.WithCancel(context.Background())

	err := components.Consumer.Start(ctx, "idempotent-consumer", broker.ConsumerConfig([]string{topic}, "idempotent-group"), func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		return kafka.AckOK()
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	cancel()
	if err := components.Consumer.Stop("idempotent-consumer"); err != nil {
		t.Fatalf("Stop after context cancel: %v", err)
	}
	if err := components.Consumer.Stop("idempotent-consumer"); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

// TestMockBroker_ConsumerCloseAfterContextCancel 验证上下文取消后 Close 可正常停止全部消费者
func TestMockBroker_ConsumerCloseAfterContextCancel(t *testing.T) {
	const topic = "close-after-cancel"

	components, broker := NewComponents(t, topic)
	ctx, cancel := context.WithCancel(context.Background())

	if err := components.Consumer.Start(ctx, "close-consumer-1", broker.ConsumerConfig([]string{topic}, "close-group-1"), func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		return kafka.AckOK()
	}); err != nil {
		t.Fatalf("Start consumer-1: %v", err)
	}
	if err := components.Consumer.Start(ctx, "close-consumer-2", broker.ConsumerConfig([]string{topic}, "close-group-2"), func(ctx context.Context, msg kafka.Message) kafka.ConsumeResult {
		return kafka.AckOK()
	}); err != nil {
		t.Fatalf("Start consumer-2: %v", err)
	}

	cancel()
	if err := components.Consumer.Close(); err != nil {
		t.Fatalf("Close after context cancel: %v", err)
	}
}

// TestMockBroker_NewWriter 验证 NewWriter 创建的 Writer 可正常写入并被 Reader 读取
func TestMockBroker_NewWriter(t *testing.T) {
	const topic = "writer-topic"

	broker := NewMockBroker(t, SeedTopics(1, topic))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	writer := broker.NewWriter(topic)
	defer func() { _ = writer.Close() }()

	if err := writer.WriteMessages(ctx, kafkago.Message{Value: []byte("published")}); err != nil {
		t.Fatalf("WriteMessages: %v", err)
	}

	reader := broker.NewReader(kafkago.ReaderConfig{
		Topic:     topic,
		Partition: 0,
	})
	defer func() { _ = reader.Close() }()

	msg, err := reader.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(msg.Value) != "published" {
		t.Fatalf("got %q, want published", msg.Value)
	}
}
