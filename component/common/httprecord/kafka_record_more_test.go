// HTTP 访问记录 Kafka 发布服务与消费者的单元/端到端测试
//
// @author zwei
package httprecord

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	yomikafka "github.com/hwwadmin/yomi-boot/component/common/kafka"
	"github.com/hwwadmin/yomi-boot/component/common/kafka/kafkatest"

	kafkago "github.com/segmentio/kafka-go"
)

// recordingKafkaPublisher 记录 PublishAsync 调用的发布器桩
type recordingKafkaPublisher struct {
	mu      sync.Mutex
	reqs    []yomikafka.PublishRequest
	err     error
	panicOn bool
}

// Publish 空实现
func (p *recordingKafkaPublisher) Publish(context.Context, yomikafka.PublishRequest) error {
	return nil
}

// PublishBatch 空实现
func (p *recordingKafkaPublisher) PublishBatch(context.Context, []yomikafka.PublishRequest) error {
	return nil
}

// PublishJSON 空实现
func (p *recordingKafkaPublisher) PublishJSON(context.Context, string, string, any) error {
	return nil
}

// PublishAsync 记录请求，按配置返回错误或 panic
func (p *recordingKafkaPublisher) PublishAsync(_ context.Context, req yomikafka.PublishRequest) error {
	if p.panicOn {
		panic("kafka boom")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reqs = append(p.reqs, req)
	return p.err
}

// PublishBatchAsync 空实现
func (p *recordingKafkaPublisher) PublishBatchAsync(context.Context, []yomikafka.PublishRequest) error {
	return nil
}

// PublishJSONAsync 空实现
func (p *recordingKafkaPublisher) PublishJSONAsync(context.Context, string, string, any) error {
	return nil
}

// Close 空实现
func (p *recordingKafkaPublisher) Close() error { return nil }

// snapshot 返回已记录请求的副本
func (p *recordingKafkaPublisher) snapshot() []yomikafka.PublishRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]yomikafka.PublishRequest(nil), p.reqs...)
}

// TestNewKafkaHttpRecordServicePanics 验证必填依赖缺失时 Fail Fast
func TestNewKafkaHttpRecordServicePanics(t *testing.T) {
	cases := []struct {
		name      string
		publisher yomikafka.PublisherService
		topic     string
	}{
		{name: "publisher 为空", publisher: nil, topic: "t"},
		{name: "topic 为空", publisher: &recordingKafkaPublisher{}, topic: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s 应 panic", tc.name)
				}
			}()
			NewKafkaHttpRecordService(tc.publisher, tc.topic)
		})
	}
}

// TestKafkaHttpRecordServiceHandle 验证发布内容组装与异常容忍
func TestKafkaHttpRecordServiceHandle(t *testing.T) {
	t.Run("正常发布且 TraceID 作为消息键", func(t *testing.T) {
		pub := &recordingKafkaPublisher{}
		svc := NewKafkaHttpRecordService(pub, "http.record")

		svc.Handle(context.Background(), &HttpRecord{
			TraceID: "tr-1", Method: "GET", Path: "/ping", StatusCode: 200, RequestHeaders: map[string]string{"k": "v"},
		})

		reqs := pub.snapshot()
		if len(reqs) != 1 {
			t.Fatalf("期望发布 1 次，实际 %d 次", len(reqs))
		}
		if reqs[0].Topic != "http.record" || string(reqs[0].Key) != "tr-1" {
			t.Fatalf("发布请求不符: %+v", reqs[0])
		}
		var wire wireRecord
		if err := json.Unmarshal(reqs[0].Value, &wire); err != nil {
			t.Fatalf("消息体应为合法 wire JSON: %v", err)
		}
		if wire.TraceID != "tr-1" || wire.Path != "/ping" || wire.StatusCode != 200 || wire.RequestHeaders["k"] != "v" {
			t.Fatalf("wire 字段不符: %+v", wire)
		}
	})

	t.Run("nil 记录不发布", func(t *testing.T) {
		pub := &recordingKafkaPublisher{}
		svc := NewKafkaHttpRecordService(pub, "http.record")
		svc.Handle(context.Background(), nil)
		if got := len(pub.snapshot()); got != 0 {
			t.Fatalf("nil 记录不应发布，实际 %d 次", got)
		}
	})

	t.Run("发布失败仅告警", func(t *testing.T) {
		pub := &recordingKafkaPublisher{err: context.Canceled}
		svc := NewKafkaHttpRecordService(pub, "http.record")
		svc.Handle(context.Background(), &HttpRecord{TraceID: "tr-2", Method: "GET", Path: "/x"})
		if got := len(pub.snapshot()); got != 1 {
			t.Fatalf("失败也应尝试发布，实际 %d 次", got)
		}
	})

	t.Run("发布 panic 被捕获", func(t *testing.T) {
		pub := &recordingKafkaPublisher{panicOn: true}
		svc := NewKafkaHttpRecordService(pub, "http.record")
		svc.Handle(context.Background(), &HttpRecord{TraceID: "tr-3", Method: "GET", Path: "/x"})
	})
}

// TestStartKafkaRecordConsumerGuards 验证消费者启动的参数校验
func TestStartKafkaRecordConsumerGuards(t *testing.T) {
	validCfg := KafkaRecordConsumerConfig{Topics: []string{"t"}, GroupID: "g"}
	handler := func(context.Context, *HttpRecord) RecordHandleResult { return HandleOK() }

	if err := StartKafkaRecordConsumer(context.Background(), nil, "id", validCfg, handler); err == nil {
		t.Fatal("nil 消费者应返回错误")
	}
	manager := &stubConsumerManager{}
	if err := StartKafkaRecordConsumer(context.Background(), manager, "id", validCfg, nil); err == nil {
		t.Fatal("nil handler 应返回错误")
	}
	if err := StartKafkaRecordConsumer(context.Background(), manager, "id", KafkaRecordConsumerConfig{GroupID: "g"}, handler); err == nil {
		t.Fatal("空主题应返回错误")
	}
	if err := StartKafkaRecordConsumer(context.Background(), manager, "id", KafkaRecordConsumerConfig{Topics: []string{"t"}}, handler); err == nil {
		t.Fatal("空消费组应返回错误")
	}
}

// TestKafkaRecordConsumerMaxRetries 验证最大重试次数的默认值、透传与负值校验
func TestKafkaRecordConsumerMaxRetries(t *testing.T) {
	handler := func(context.Context, *HttpRecord) RecordHandleResult { return HandleOK() }
	cases := []struct {
		name       string
		configured *int
		want       int
	}{
		{name: "未配置使用默认值", want: defaultKafkaRecordMaxRetries},
		{name: "显式 0 不重试", configured: intPtr(0), want: 0},
		{name: "显式 5", configured: intPtr(5), want: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager := &stubConsumerManager{}
			cfg := KafkaRecordConsumerConfig{Topics: []string{"t"}, GroupID: "g", MaxRetries: tc.configured}
			if err := StartKafkaRecordConsumer(context.Background(), manager, "id", cfg, handler); err != nil {
				t.Fatalf("StartKafkaRecordConsumer() error = %v", err)
			}
			if manager.cfg.MaxHandlerRetries == nil {
				t.Fatal("MaxHandlerRetries 未透传")
			}
			if got := *manager.cfg.MaxHandlerRetries; got != tc.want {
				t.Fatalf("MaxHandlerRetries = %d，期望 %d", got, tc.want)
			}
		})
	}

	if err := StartKafkaRecordConsumer(context.Background(), &stubConsumerManager{}, "id",
		KafkaRecordConsumerConfig{Topics: []string{"t"}, GroupID: "g", MaxRetries: intPtr(-1)}, handler); err == nil {
		t.Fatal("负最大重试次数应返回错误")
	}
}

// stubConsumerManager 记录 Start 调用的消费者管理器桩
type stubConsumerManager struct {
	started int
	cfg     yomikafka.ConsumerConfig
}

// Start 记录调用与消费配置并返回 nil
func (m *stubConsumerManager) Start(_ context.Context, _ string, cfg yomikafka.ConsumerConfig, _ yomikafka.ConsumeHandler) error {
	m.started++
	m.cfg = cfg
	return nil
}

// Stop 空实现
func (m *stubConsumerManager) Stop(string) error { return nil }

// StopAll 空实现
func (m *stubConsumerManager) StopAll() error { return nil }

// Close 空实现
func (m *stubConsumerManager) Close() error { return nil }

// TestKafkaRecordE2E 基于 kfake 内存集群验证发布→消费→解析全链路
func TestKafkaRecordE2E(t *testing.T) {
	broker := kafkatest.NewMockBroker(t, kafkatest.SeedTopics(1, "httprecord.e2e"))
	manager := kafkatest.NewConsumerManager(t, broker)
	publisher := kafkatest.NewPublisher(t, broker)

	svc := NewKafkaHttpRecordService(publisher, "httprecord.e2e")
	collected := make(chan *HttpRecord, 8)
	MustStartKafkaRecordConsumer(context.Background(), manager, KafkaConsumerID,
		KafkaRecordConsumerConfig{Topics: []string{"httprecord.e2e"}, GroupID: "httprecord-e2e-group"},
		func(_ context.Context, record *HttpRecord) RecordHandleResult {
			collected <- record
			return HandleOK()
		})

	svc.Handle(context.Background(), &HttpRecord{
		TraceID: "e2e-1", Method: "POST", Path: "/api/x", StatusCode: 200, UserID: 3, ClientType: "app",
	})

	select {
	case record := <-collected:
		if record.TraceID != "e2e-1" || record.Path != "/api/x" || record.Method != "POST" {
			t.Fatalf("消费记录不符: %+v", record)
		}
		if record.UserID != 3 || record.ClientType != "app" || record.StatusCode != 200 {
			t.Fatalf("消费记录字段不符: %+v", record)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("等待消费记录超时")
	}

	// 非法 JSON 消息应被丢弃且不影响后续消费
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := broker.Publish(ctx, "httprecord.e2e", kafkago.Message{Value: []byte("not-json")}); err != nil {
		t.Fatalf("写入非法消息失败: %v", err)
	}
	svc.Handle(context.Background(), &HttpRecord{TraceID: "e2e-2", Method: "GET", Path: "/api/y"})

	select {
	case record := <-collected:
		if record.TraceID != "e2e-2" {
			t.Fatalf("非法消息后应继续消费有效记录，实际 %+v", record)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("等待第二条消费记录超时")
	}
}

// TestMustStartKafkaRecordConsumerPanics 验证 Fail Fast
func TestMustStartKafkaRecordConsumerPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil 消费者应 panic")
		}
	}()
	MustStartKafkaRecordConsumer(context.Background(), nil, "id",
		KafkaRecordConsumerConfig{Topics: []string{"t"}, GroupID: "g"},
		func(context.Context, *HttpRecord) RecordHandleResult { return HandleOK() })
}

// blockingKafkaPublisher 阻塞到投递上下文结束的发布器桩
type blockingKafkaPublisher struct{}

// Publish 空实现
func (blockingKafkaPublisher) Publish(context.Context, yomikafka.PublishRequest) error { return nil }

// PublishBatch 空实现
func (blockingKafkaPublisher) PublishBatch(context.Context, []yomikafka.PublishRequest) error {
	return nil
}

// PublishJSON 空实现
func (blockingKafkaPublisher) PublishJSON(context.Context, string, string, any) error {
	return nil
}

// PublishAsync 阻塞到投递上下文结束
func (blockingKafkaPublisher) PublishAsync(ctx context.Context, _ yomikafka.PublishRequest) error {
	<-ctx.Done()
	return ctx.Err()
}

// PublishBatchAsync 空实现
func (blockingKafkaPublisher) PublishBatchAsync(context.Context, []yomikafka.PublishRequest) error {
	return nil
}

// PublishJSONAsync 空实现
func (blockingKafkaPublisher) PublishJSONAsync(context.Context, string, string, any) error {
	return nil
}

// Close 空实现
func (blockingKafkaPublisher) Close() error { return nil }

// TestKafkaRecordHandleBudget 验证发布阻塞时投递只等独立短预算，且不受调用方 ctx 取消影响
func TestKafkaRecordHandleBudget(t *testing.T) {
	svc := NewKafkaHttpRecordService(blockingKafkaPublisher{}, "t")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	svc.Handle(ctx, &HttpRecord{TraceID: "budget", Method: "GET", Path: "/x"})
	elapsed := time.Since(start)

	if elapsed < recordPublishBudget/2 {
		t.Fatalf("调用方 ctx 已取消不应缩短投递预算，实际耗时 %v", elapsed)
	}
	if elapsed > recordPublishBudget+2*time.Second {
		t.Fatalf("投递阻塞超出预算: %v", elapsed)
	}
}
