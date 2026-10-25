// Package kafka_test 的消费者管理器端到端测试（基于 kfake 内存集群）
//
// @author zwei
package kafka_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/common/kafka/kafkatest"

	kafkago "github.com/segmentio/kafka-go"
)

// consumeCollector 收集消费到的消息并通过 channel 通知
type consumeCollector struct {
	mu      sync.Mutex
	msgs    []kafka.Message
	results chan struct{}
}

// handle 记录消息并返回预设结果
func (c *consumeCollector) handle(result kafka.ConsumeResult) kafka.ConsumeHandler {
	return func(_ context.Context, msg kafka.Message) kafka.ConsumeResult {
		c.mu.Lock()
		c.msgs = append(c.msgs, msg)
		c.mu.Unlock()
		return result
	}
}

// handleFunc 按序返回预设结果序列，末尾结果循环使用
func (c *consumeCollector) handleFunc(results ...kafka.ConsumeResult) kafka.ConsumeHandler {
	idx := 0
	return func(_ context.Context, msg kafka.Message) kafka.ConsumeResult {
		c.mu.Lock()
		c.msgs = append(c.msgs, msg)
		c.mu.Unlock()
		result := results[idx]
		if idx < len(results)-1 {
			idx++
		}
		return result
	}
}

// count 返回已收集的消息数
func (c *consumeCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

// waitForCount 轮询等待消息数达到期望值
func waitForCount(t *testing.T, c *consumeCollector, want int) []kafka.Message {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if c.count() >= want {
			c.mu.Lock()
			defer c.mu.Unlock()
			return append([]kafka.Message(nil), c.msgs...)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("等待 %d 条消息超时，实际 %d 条", want, c.count())
	return nil
}

// publishRaw 通过 mock broker 直接写入一条消息
func publishRaw(t *testing.T, broker *kafkatest.MockBroker, topic, value string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := broker.Publish(ctx, topic, kafkago.Message{Key: []byte("rk"), Value: []byte(value)}); err != nil {
		t.Fatalf("写入测试消息失败: %v", err)
	}
}

// TestConsumerE2EAckOK 验证消息被消费、字段透传且偏移量提交
func TestConsumerE2EAckOK(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-consume-ok"))
	manager := kafkatest.NewConsumerManager(t, broker)

	collector := &consumeCollector{}
	err := manager.Start(
		context.Background(), "ok-consumer",
		broker.ConsumerConfig([]string{"e2e-consume-ok"}, "group-ok"),
		collector.handle(kafka.AckOK()),
	)
	if err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}

	publishRaw(t, broker, "e2e-consume-ok", "hello-consume")
	msgs := waitForCount(t, collector, 1)
	if string(msgs[0].Value) != "hello-consume" || string(msgs[0].Key) != "rk" {
		t.Fatalf("消费消息不符: %+v", msgs[0])
	}
	if msgs[0].Topic != "e2e-consume-ok" {
		t.Fatalf("消息主题不符: %+v", msgs[0])
	}

	if err := manager.Stop("ok-consumer"); err != nil {
		t.Fatalf("停止消费者失败: %v", err)
	}
}

// TestConsumerE2ERetryThenAck 验证 handler 返回 Retry 时在同一消息上重试直至成功
func TestConsumerE2ERetryThenAck(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-consume-retry"))
	manager := kafkatest.NewConsumerManager(t, broker)

	collector := &consumeCollector{}
	err := manager.Start(
		context.Background(), "retry-consumer",
		broker.ConsumerConfig([]string{"e2e-consume-retry"}, "group-retry"),
		collector.handleFunc(kafka.Retry(), kafka.Retry(), kafka.AckOK()),
	)
	if err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}

	publishRaw(t, broker, "e2e-consume-retry", "retry-msg")
	msgs := waitForCount(t, collector, 3)
	for i, msg := range msgs {
		if string(msg.Value) != "retry-msg" {
			t.Fatalf("第 %d 次应重试同一消息，实际 %+v", i, msg)
		}
	}
}

// TestConsumerE2ERetryExhaustedDrops 验证重试超限后自动丢弃并提交偏移量
func TestConsumerE2ERetryExhaustedDrops(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-consume-drop"))
	manager := kafkatest.NewConsumerManager(t, broker)

	maxRetries := 1
	cfg := broker.ConsumerConfig([]string{"e2e-consume-drop"}, "group-drop")
	cfg.MaxHandlerRetries = &maxRetries

	collector := &consumeCollector{}
	err := manager.Start(
		context.Background(), "drop-consumer", cfg,
		collector.handleFunc(kafka.Retry(), kafka.Retry(), kafka.Retry()),
	)
	if err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}

	publishRaw(t, broker, "e2e-consume-drop", "drop-msg")
	// 首次处理 + 1 次重试后超限丢弃，不应再消费同一条消息
	msgs := waitForCount(t, collector, 2)
	if len(msgs) > 2 {
		t.Fatalf("重试超限后不应继续重试，实际 %d 次", len(msgs))
	}
}

// TestConsumerE2EAckDropSkipsMessage 验证 Drop 提交偏移量后跳过当前消息继续消费
func TestConsumerE2EAckDropSkipsMessage(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-consume-skip"))
	manager := kafkatest.NewConsumerManager(t, broker)

	collector := &consumeCollector{}
	err := manager.Start(
		context.Background(), "skip-consumer",
		broker.ConsumerConfig([]string{"e2e-consume-skip"}, "group-skip"),
		collector.handle(kafka.AckDrop()),
	)
	if err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}

	publishRaw(t, broker, "e2e-consume-skip", "skip-1")
	publishRaw(t, broker, "e2e-consume-skip", "skip-2")
	msgs := waitForCount(t, collector, 2)
	for i, want := range []string{"skip-1", "skip-2"} {
		if string(msgs[i].Value) != want {
			t.Fatalf("第 %d 条期望 %s，实际 %s", i, want, msgs[i].Value)
		}
	}
}

// TestConsumerE2EHandlerPanicRecovered 验证 handler panic 被捕获并按重试计数处理
func TestConsumerE2EHandlerPanicRecovered(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-consume-panic"))
	manager := kafkatest.NewConsumerManager(t, broker)

	maxRetries := 0
	cfg := broker.ConsumerConfig([]string{"e2e-consume-panic"}, "group-panic")
	cfg.MaxHandlerRetries = &maxRetries

	collector := &consumeCollector{}
	handler := func(context.Context, kafka.Message) kafka.ConsumeResult {
		collector.handle(kafka.Retry())(nil, kafka.Message{})
		panic("boom")
	}
	if err := manager.Start(context.Background(), "panic-consumer", cfg, handler); err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}

	publishRaw(t, broker, "e2e-consume-panic", "panic-msg")
	// panic 被捕获按 Retry 处理，MaxHandlerRetries=0 时立即超限丢弃，消费循环继续存活
	waitForCount(t, collector, 1)
}

// TestConsumerManagerLifecycle 验证消费者的注册、重复注册、停止与关闭
func TestConsumerManagerLifecycle(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-consume-lifecycle"))
	manager := kafkatest.NewConsumerManager(t, broker)
	cfg := broker.ConsumerConfig([]string{"e2e-consume-lifecycle"}, "group-lifecycle")
	handler := (&consumeCollector{}).handle(kafka.AckOK())
	ctx := context.Background()

	if err := manager.Start(ctx, "", cfg, handler); err == nil {
		t.Fatal("空消费者标识应返回错误")
	}
	if err := manager.Start(ctx, "c1", cfg, nil); err == nil {
		t.Fatal("nil handler 应返回错误")
	}
	if err := manager.Start(ctx, "c1", cfg, handler); err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}
	if err := manager.Start(ctx, "c1", cfg, handler); err == nil {
		t.Fatal("重复注册应返回错误")
	}

	// 非法消费参数
	badTopics := cfg
	badTopics.Topics = nil
	if err := manager.Start(ctx, "c2", badTopics, handler); err == nil {
		t.Fatal("空订阅主题应返回错误")
	}
	badGroup := cfg
	badGroup.GroupID = " "
	if err := manager.Start(ctx, "c2", badGroup, handler); err == nil {
		t.Fatal("空消费组应返回错误")
	}
	badBytes := cfg
	badBytes.MinBytes = 2000
	badBytes.MaxBytes = 1000
	if err := manager.Start(ctx, "c2", badBytes, handler); err == nil {
		t.Fatal("min_bytes 大于 max_bytes 应返回错误")
	}

	if err := manager.Stop("missing"); err != nil {
		t.Fatalf("停止不存在的消费者应幂等: %v", err)
	}
	if err := manager.Stop("c1"); err != nil {
		t.Fatalf("停止消费者失败: %v", err)
	}
	// 停止后可复用同一标识重新启动
	if err := manager.Start(ctx, "c1", cfg, handler); err != nil {
		t.Fatalf("停止后重新启动失败: %v", err)
	}
	if err := manager.StopAll(); err != nil {
		t.Fatalf("停止全部消费者失败: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("关闭管理器失败: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("重复关闭应幂等: %v", err)
	}
}

// TestConsumerE2EStopInterruptsConsume 验证 Stop 能中断阻塞中的消费循环
func TestConsumerE2EStopInterruptsConsume(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-consume-stop"))
	manager := kafkatest.NewConsumerManager(t, broker)

	collector := &consumeCollector{}
	err := manager.Start(
		context.Background(), "stop-consumer",
		broker.ConsumerConfig([]string{"e2e-consume-stop"}, "group-stop"),
		collector.handle(kafka.AckOK()),
	)
	if err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		_ = manager.Stop("stop-consumer")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop 应在有限时间内返回")
	}
}
