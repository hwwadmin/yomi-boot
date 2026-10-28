// HTTP 访问记录插件的消费装配与生命周期测试。

package httprecordplugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/traceid"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// newTestConfig 加载仅含 http_record 段的测试配置
func newTestConfig(t *testing.T, content string) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	reg := config.NewRegistry()
	reg.Register(ConfigSection())
	return config.MustLoadConfig(path, "", reg)
}

// mustPanic 断言 fn 触发 panic
func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("期望 panic，实际未触发")
		}
	}()
	fn()
}

func boolPtr(v bool) *bool { return &v }

// fakeRecordService 是仅用于装配校验的生产端空实现
type fakeRecordService struct{}

// Handle 空实现
func (fakeRecordService) Handle(context.Context, *httprecord.HttpRecord) {}

// fakeKafkaConsumer 记录 Kafka 消费端的启停调用
type fakeKafkaConsumer struct {
	started  bool
	id       string
	cfg      yomikafka.ConsumerConfig
	stopped  string
	startErr error
	stopErr  error
}

// Start 记录启动参数
func (f *fakeKafkaConsumer) Start(_ context.Context, id string, cfg yomikafka.ConsumerConfig, _ yomikafka.ConsumeHandler) error {
	f.started = true
	f.id = id
	f.cfg = cfg
	return f.startErr
}

// Stop 记录停止标识
func (f *fakeKafkaConsumer) Stop(id string) error {
	f.stopped = id
	return f.stopErr
}

// StopAll 空实现
func (f *fakeKafkaConsumer) StopAll() error { return nil }

// Close 空实现
func (f *fakeKafkaConsumer) Close() error { return nil }

// fakeRedisStreamConsumer 记录 Redis Stream 消费端的启停调用
type fakeRedisStreamConsumer struct {
	started bool
	id      string
	cfg     redisstream.GroupConfig
	stopped string
}

// Start 记录启动参数
func (f *fakeRedisStreamConsumer) Start(_ context.Context, id string, cfg redisstream.GroupConfig, _ redisstream.Handler) error {
	f.started = true
	f.id = id
	f.cfg = cfg
	return nil
}

// Stop 记录停止标识
func (f *fakeRedisStreamConsumer) Stop(id string) error {
	f.stopped = id
	return nil
}

// StopAll 空实现
func (f *fakeRedisStreamConsumer) StopAll() error { return nil }

// Close 空实现
func (f *fakeRedisStreamConsumer) Close() error { return nil }

// TestMemoryConsumerRoundTrip 验证 memory provider 自动启动消费并注入 TraceID
func TestMemoryConsumerRoundTrip(t *testing.T) {
	cfg := newTestConfig(t, "http_record:\n  enabled: true\n  provider: memory\n  memory:\n    workers: 1\n")
	p := NewHttpRecord()
	slot := plugin.NewPluginSlot(cfg, []plugin.Plugin{p})
	ctx := context.Background()
	if err := slot.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	records := make(chan *httprecord.HttpRecord, 1)
	p.MustRegisterHandler(func(ctx context.Context, record *httprecord.HttpRecord) httprecord.RecordHandleResult {
		if got := traceid.FromContext(ctx); got != record.TraceID {
			t.Errorf("消费上下文 TraceID = %q，期望 %q", got, record.TraceID)
		}
		records <- record
		return httprecord.HandleOK()
	})
	if err := p.OnReady(ctx); err != nil {
		t.Fatalf("OnReady() error = %v", err)
	}

	p.Service().Handle(ctx, &httprecord.HttpRecord{TraceID: "trace-1", Path: "/demo"})
	select {
	case record := <-records:
		if record.TraceID != "trace-1" {
			t.Fatalf("消费记录 TraceID = %q，期望 trace-1", record.TraceID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("内存消费者未处理记录")
	}

	if err := slot.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if p.Service() != nil {
		t.Fatal("Close 后 Service 应为 nil")
	}
}

// TestOnReadyRequiresHandler 验证启用消费但未注册处理函数时启动失败
func TestOnReadyRequiresHandler(t *testing.T) {
	cfg := newTestConfig(t, "http_record:\n  enabled: true\n  provider: memory\n")
	p := NewHttpRecord()
	slot := plugin.NewPluginSlot(cfg, []plugin.Plugin{p})
	ctx := context.Background()
	if err := slot.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() { _ = slot.Close(ctx) }()

	if err := p.OnReady(ctx); err == nil {
		t.Fatal("未注册 RecordHandler 时 OnReady 应返回错误")
	}
}

// TestDisabledPlugin 验证未启用时插件被跳过且服务为空
func TestDisabledPlugin(t *testing.T) {
	cfg := newTestConfig(t, "http_record:\n  enabled: false\n")
	p := NewHttpRecord()
	slot := plugin.NewPluginSlot(cfg, []plugin.Plugin{p})
	ctx := context.Background()
	if err := slot.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if slot.Started(Name) {
		t.Fatal("未启用时插件不应被标记为已启动")
	}
	if p.Service() != nil {
		t.Fatal("未启用时 Service 应为 nil")
	}
}

// TestMustRegisterHandlerPanics 验证 nil 与重复注册触发 panic
func TestMustRegisterHandlerPanics(t *testing.T) {
	p := NewHttpRecord()
	mustPanic(t, func() { p.MustRegisterHandler(nil) })

	handler := func(context.Context, *httprecord.HttpRecord) httprecord.RecordHandleResult {
		return httprecord.HandleOK()
	}
	p.MustRegisterHandler(handler)
	mustPanic(t, func() { p.MustRegisterHandler(handler) })

	var nilPlugin *HttpRecord
	mustPanic(t, func() { nilPlugin.MustRegisterHandler(handler) })
}

// TestKafkaConsumerLifecycle 验证 kafka provider 在 OnReady 启动、Close 停止
func TestKafkaConsumerLifecycle(t *testing.T) {
	consumer := &fakeKafkaConsumer{}
	p := &HttpRecord{
		Base:          plugin.NewBase(Name),
		service:       fakeRecordService{},
		cfg:           httprecord.Config{KafkaTopic: "topic-1", Consumer: httprecord.ConsumerConfig{Enabled: boolPtr(true), GroupID: "group-1"}},
		provider:      httprecord.ProviderKafka,
		runtimeCtx:    context.Background(),
		kafkaConsumer: consumer,
	}
	p.MustRegisterHandler(func(context.Context, *httprecord.HttpRecord) httprecord.RecordHandleResult {
		return httprecord.HandleOK()
	})

	if err := p.OnReady(context.Background()); err != nil {
		t.Fatalf("OnReady() error = %v", err)
	}
	if !consumer.started {
		t.Fatal("OnReady 应启动 kafka 消费者")
	}
	if consumer.id != httprecord.KafkaConsumerID {
		t.Fatalf("消费者标识 = %q，期望 %q", consumer.id, httprecord.KafkaConsumerID)
	}
	if len(consumer.cfg.Topics) != 1 || consumer.cfg.Topics[0] != "topic-1" {
		t.Fatalf("订阅主题 = %v，期望 [topic-1]", consumer.cfg.Topics)
	}
	if consumer.cfg.GroupID != "group-1" {
		t.Fatalf("消费组 = %q，期望 group-1", consumer.cfg.GroupID)
	}

	if err := p.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if consumer.stopped != httprecord.KafkaConsumerID {
		t.Fatalf("停止标识 = %q，期望 %q", consumer.stopped, httprecord.KafkaConsumerID)
	}
}

// TestRedisStreamConsumerLifecycle 验证 redis_stream provider 在 OnReady 启动、Close 停止
func TestRedisStreamConsumerLifecycle(t *testing.T) {
	consumer := &fakeRedisStreamConsumer{}
	p := &HttpRecord{
		Base:           plugin.NewBase(Name),
		service:        fakeRecordService{},
		cfg:            httprecord.Config{Stream: "stream-1", Consumer: httprecord.ConsumerConfig{Enabled: boolPtr(true), GroupID: "group-1"}},
		provider:       httprecord.ProviderRedisStream,
		runtimeCtx:     context.Background(),
		streamConsumer: consumer,
		consumerName:   "demo-1",
	}
	p.MustRegisterHandler(func(context.Context, *httprecord.HttpRecord) httprecord.RecordHandleResult {
		return httprecord.HandleOK()
	})

	if err := p.OnReady(context.Background()); err != nil {
		t.Fatalf("OnReady() error = %v", err)
	}
	if !consumer.started {
		t.Fatal("OnReady 应启动 Redis Stream 消费者")
	}
	if consumer.id != httprecord.RedisStreamConsumerID {
		t.Fatalf("消费者标识 = %q，期望 %q", consumer.id, httprecord.RedisStreamConsumerID)
	}
	if consumer.cfg.Stream != "stream-1" || consumer.cfg.Group != "group-1" || consumer.cfg.Consumer != "demo-1" {
		t.Fatalf("消费组参数 = %+v", consumer.cfg)
	}

	if err := p.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if consumer.stopped != httprecord.RedisStreamConsumerID {
		t.Fatalf("停止标识 = %q，期望 %q", consumer.stopped, httprecord.RedisStreamConsumerID)
	}
}

// TestConsumerDisabledByConfig 验证 durable provider 未启用消费时不启动消费者
func TestConsumerDisabledByConfig(t *testing.T) {
	consumer := &fakeKafkaConsumer{}
	p := &HttpRecord{
		Base:          plugin.NewBase(Name),
		service:       fakeRecordService{},
		cfg:           httprecord.Config{KafkaTopic: "topic-1", Consumer: httprecord.ConsumerConfig{Enabled: boolPtr(false)}},
		provider:      httprecord.ProviderKafka,
		runtimeCtx:    context.Background(),
		kafkaConsumer: consumer,
	}
	p.MustRegisterHandler(func(context.Context, *httprecord.HttpRecord) httprecord.RecordHandleResult {
		return httprecord.HandleOK()
	})

	if err := p.OnReady(context.Background()); err != nil {
		t.Fatalf("OnReady() error = %v", err)
	}
	if consumer.started {
		t.Fatal("consumer.enabled=false 时不应启动消费者")
	}
}

// TestInitRequiresKafkaPublisher 验证 provider=kafka 缺少发布端插件时启动失败
func TestInitRequiresKafkaPublisher(t *testing.T) {
	cfg := newTestConfig(t, "http_record:\n  enabled: true\n  provider: kafka\n")
	p := NewHttpRecord()
	slot := plugin.NewPluginSlot(cfg, []plugin.Plugin{p})
	if err := slot.Init(context.Background()); err == nil {
		t.Fatal("缺少 kafka-publisher 插件时 Init 应返回错误")
	}
}

// TestRedisConsumerName 验证无 appctx 时消费者名退化为进程级唯一名
func TestRedisConsumerName(t *testing.T) {
	name := redisConsumerName(nil)
	if !strings.HasPrefix(name, "httprecord-") {
		t.Fatalf("消费者名 = %q，期望以 httprecord- 开头", name)
	}
}

// TestOnReadyStartError 验证消费启动失败向上返回
func TestOnReadyStartError(t *testing.T) {
	consumer := &fakeKafkaConsumer{startErr: errors.New("broker down")}
	p := &HttpRecord{
		Base:          plugin.NewBase(Name),
		service:       fakeRecordService{},
		cfg:           httprecord.Config{Consumer: httprecord.ConsumerConfig{Enabled: boolPtr(true)}},
		provider:      httprecord.ProviderKafka,
		runtimeCtx:    context.Background(),
		kafkaConsumer: consumer,
	}
	p.MustRegisterHandler(func(context.Context, *httprecord.HttpRecord) httprecord.RecordHandleResult {
		return httprecord.HandleOK()
	})

	err := p.OnReady(context.Background())
	if err == nil || !strings.Contains(err.Error(), "启动消费者失败") {
		t.Fatalf("OnReady() error = %v，期望启动失败", err)
	}
}

// TestCloseAggregatesStopError 验证停止失败仍完成资源清理并返回错误
func TestCloseAggregatesStopError(t *testing.T) {
	consumer := &fakeKafkaConsumer{stopErr: errors.New("stop failed")}
	p := &HttpRecord{service: fakeRecordService{}, kafkaConsumer: consumer}

	if err := p.Close(context.Background()); err == nil {
		t.Fatal("Stop 失败时 Close 应返回错误")
	}
	if p.Service() != nil {
		t.Fatal("Close 后 Service 应为 nil")
	}
}

// TestFromAndNilService 验证空插槽与 nil 插件的访问语义
func TestFromAndNilService(t *testing.T) {
	if From(nil) != nil {
		t.Fatal("From(nil) 应返回 nil")
	}

	var p *HttpRecord
	if p.Service() != nil {
		t.Fatal("nil 插件 Service 应为 nil")
	}
	if err := p.Close(context.Background()); err != nil {
		t.Fatalf("nil 插件 Close() error = %v", err)
	}
}

// TestInitRequiresRedisStreamPublisher 验证 provider=redis_stream 缺少发布端插件时启动失败
func TestInitRequiresRedisStreamPublisher(t *testing.T) {
	cfg := newTestConfig(t, "http_record:\n  enabled: true\n  provider: redis_stream\n")
	p := NewHttpRecord()
	slot := plugin.NewPluginSlot(cfg, []plugin.Plugin{p})
	if err := slot.Init(context.Background()); err == nil {
		t.Fatal("缺少 redisstream 插件时 Init 应返回错误")
	}
}

// TestUnsupportedProviderBranches 验证生产端与消费端对未知 provider 的防御分支
func TestUnsupportedProviderBranches(t *testing.T) {
	p := &HttpRecord{
		provider: "nsq",
		cfg:      httprecord.Config{Consumer: httprecord.ConsumerConfig{Enabled: boolPtr(true)}},
	}
	if err := p.initProducer(nil); err == nil {
		t.Fatal("未知 provider 装配生产端应返回错误")
	}
	if err := p.initConsumerDeps(nil); err == nil {
		t.Fatal("未知 provider 装配消费端应返回错误")
	}
}
