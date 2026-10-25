// Package kafka_test 的生产者端到端测试（基于 kfake 内存集群）
//
// @author zwei
package kafka_test

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/common/kafka/kafkatest"

	kafkago "github.com/segmentio/kafka-go"
)

// readOne 用裸 Reader 读取一条消息，超时判定失败
func readOne(t *testing.T, broker *kafkatest.MockBroker, topic string) kafkago.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reader := broker.NewReader(kafkago.ReaderConfig{Topic: topic})
	defer func() { _ = reader.Close() }()

	msg, err := reader.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}
	return msg
}

// readMany 用同一个 Reader 顺序读取 n 条消息，保证偏移量连续推进
func readMany(t *testing.T, broker *kafkatest.MockBroker, topic string, n int) []kafkago.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reader := broker.NewReader(kafkago.ReaderConfig{Topic: topic})
	defer func() { _ = reader.Close() }()

	msgs := make([]kafkago.Message, 0, n)
	for i := 0; i < n; i++ {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			t.Fatalf("读取第 %d 条消息失败: %v", i+1, err)
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// TestPublisherPublishE2E 验证同步发布单条消息可被消费
func TestPublisherPublishE2E(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-publish"))
	publisher := kafkatest.NewPublisher(t, broker)
	ctx := context.Background()

	if err := publisher.Publish(ctx, kafka.PublishRequest{
		Topic: "e2e-publish",
		Key:   []byte("k1"),
		Value: []byte("v1"),
		Headers: []kafka.Header{
			{Key: "trace", Value: []byte("t1")},
		},
	}); err != nil {
		t.Fatalf("发布失败: %v", err)
	}

	msg := readOne(t, broker, "e2e-publish")
	if string(msg.Key) != "k1" || string(msg.Value) != "v1" {
		t.Fatalf("消息内容不符: %+v", msg)
	}
	if len(msg.Headers) != 1 || msg.Headers[0].Key != "trace" {
		t.Fatalf("消息头不符: %+v", msg.Headers)
	}
}

// TestPublisherPublishJSONE2E 验证 JSON 发布序列化与投递
func TestPublisherPublishJSONE2E(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-json"))
	publisher := kafkatest.NewPublisher(t, broker)

	if err := publisher.PublishJSON(context.Background(), "e2e-json", "k2", map[string]any{"n": 1}); err != nil {
		t.Fatalf("JSON 发布失败: %v", err)
	}

	msg := readOne(t, broker, "e2e-json")
	if string(msg.Key) != "k2" || string(msg.Value) != `{"n":1}` {
		t.Fatalf("JSON 消息不符: %s %s", msg.Key, msg.Value)
	}
}

// TestPublisherPublishBatchE2E 验证批量发布全部投递
func TestPublisherPublishBatchE2E(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-batch"))
	publisher := kafkatest.NewPublisher(t, broker)

	if err := publisher.PublishBatch(context.Background(), []kafka.PublishRequest{
		{Topic: "e2e-batch", Value: []byte("m1")},
		{Topic: "e2e-batch", Value: []byte("m2")},
		{Topic: "e2e-batch", Value: []byte("m3")},
	}); err != nil {
		t.Fatalf("批量发布失败: %v", err)
	}

	msgs := readMany(t, broker, "e2e-batch", 3)
	for i, want := range []string{"m1", "m2", "m3"} {
		if string(msgs[i].Value) != want {
			t.Fatalf("第 %d 条期望消息 %s，实际 %s", i, want, msgs[i].Value)
		}
	}
}

// TestPublisherPublishInvalidRequests 验证非法请求被拒绝且不投递
func TestPublisherPublishInvalidRequests(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-invalid"))
	publisher := kafkatest.NewPublisher(t, broker)
	ctx := context.Background()

	if err := publisher.Publish(ctx, kafka.PublishRequest{Topic: "", Value: []byte("v")}); err == nil {
		t.Fatal("空 topic 应返回错误")
	}
	if err := publisher.Publish(ctx, kafka.PublishRequest{Topic: "e2e-invalid", Value: nil}); err == nil {
		t.Fatal("空消息体应返回错误")
	}
	err := publisher.PublishBatch(ctx, []kafka.PublishRequest{
		{Topic: "e2e-invalid", Value: []byte("v")},
		{Topic: "e2e-invalid", Value: nil},
	})
	if err == nil {
		t.Fatal("批量含非法请求应返回错误")
	}
}

// TestPublisherPublishAsyncE2E 验证异步入队后消息最终落盘
func TestPublisherPublishAsyncE2E(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-async"))
	publisher := kafkatest.NewPublisher(t, broker)
	ctx := context.Background()

	if err := publisher.PublishAsync(ctx, kafka.PublishRequest{
		Topic: "e2e-async", Key: []byte("ak"), Value: []byte("av"),
	}); err != nil {
		t.Fatalf("异步发布失败: %v", err)
	}
	if err := publisher.PublishJSONAsync(ctx, "e2e-async", "jk", map[string]string{"a": "1"}); err != nil {
		t.Fatalf("异步 JSON 发布失败: %v", err)
	}
	if err := publisher.PublishBatchAsync(ctx, []kafka.PublishRequest{
		{Topic: "e2e-async", Value: []byte("b1")},
		{Topic: "e2e-async", Value: []byte("b2")},
	}); err != nil {
		t.Fatalf("异步批量发布失败: %v", err)
	}

	seen := map[string]bool{}
	for _, msg := range readMany(t, broker, "e2e-async", 4) {
		seen[string(msg.Value)] = true
	}
	for _, want := range []string{"av", `{"a":"1"}`, "b1", "b2"} {
		if !seen[want] {
			t.Fatalf("缺少异步消息 %s: %v", want, seen)
		}
	}
}

// TestPublisherCloseIdempotent 验证生产者可重复关闭
func TestPublisherCloseIdempotent(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-close"))
	publisher, err := kafka.NewPublisher(context.Background(), broker.PublisherConfig())
	if err != nil {
		t.Fatalf("创建生产者失败: %v", err)
	}
	if err := publisher.Close(); err != nil {
		t.Fatalf("首次关闭失败: %v", err)
	}
	if err := publisher.Close(); err != nil {
		t.Fatalf("重复关闭应幂等: %v", err)
	}
}

// TestMustNewPublisher 验证 Fail Fast 构造的成功与失败路径
func TestMustNewPublisher(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-must-pub"))
	publisher := kafkatest.NewPublisher(t, broker)
	if publisher == nil {
		t.Fatal("MustNewPublisher 应返回生产者")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("非法配置应 panic")
		}
	}()
	kafka.MustNewPublisher(context.Background(), kafka.PublisherConfig{})
}

// TestMustNewConsumerManager 验证消费者管理器 Fail Fast 构造
func TestMustNewConsumerManager(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-must-consumer"))
	manager := kafkatest.NewConsumerManager(t, broker)
	if manager == nil {
		t.Fatal("MustNewConsumerManager 应返回管理器")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("非法配置应 panic")
		}
	}()
	kafka.MustNewConsumerManager(context.Background(), kafka.ConsumerManagerConfig{})
}

// TestMustNewAdmin 验证管理端 Fail Fast 构造
func TestMustNewAdmin(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "e2e-must-admin"))
	admin := kafkatest.NewAdmin(t, broker)
	if admin == nil {
		t.Fatal("MustNewAdmin 应返回管理端")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("非法配置应 panic")
		}
	}()
	kafka.MustNewAdmin(context.Background(), kafka.ConnectionConfig{})
}
