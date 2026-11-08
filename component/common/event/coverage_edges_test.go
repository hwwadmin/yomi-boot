package event

import (
	"context"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/distlock"
	"github.com/hwwadmin/yomi-boot/component/common/event/cfg"
	"github.com/hwwadmin/yomi-boot/component/common/event/outbox"
	"github.com/hwwadmin/yomi-boot/component/common/event/repo"
	yomikafka "github.com/hwwadmin/yomi-boot/component/common/kafka"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

type edgePublisher struct{}

func (edgePublisher) Publish(context.Context, yomikafka.PublishRequest) error { return nil }
func (edgePublisher) PublishBatch(context.Context, []yomikafka.PublishRequest) error {
	return nil
}
func (edgePublisher) PublishJSON(context.Context, string, string, any) error {
	return errors.New("publish")
}
func (edgePublisher) PublishAsync(context.Context, yomikafka.PublishRequest) error { return nil }
func (edgePublisher) PublishBatchAsync(context.Context, []yomikafka.PublishRequest) error {
	return nil
}
func (edgePublisher) PublishJSONAsync(context.Context, string, string, any) error { return nil }
func (edgePublisher) Close() error                                                { return nil }

type nopLockFactory struct{}

func (nopLockFactory) GetLock(string, ...distlock.LockOption) distlock.Lock { return nil }

func TestConfigExplicitValuesAndRejects(t *testing.T) {
	if (Config{}).ServiceNameOrDefault("  ") != defaultEventServiceName {
		t.Fatal("空服务名应回落到默认值")
	}
	retry := 3
	inboxRetry := 4
	explicit := Config{
		FetchFrequencyMS:  1500,
		FetchSize:         10,
		SentThreadCount:   2,
		OutboxDlqSize:     8,
		OutboxDlqMaxRetry: &retry,
		InboxDlqMaxRetry:  &inboxRetry,
		InboxDlqSize:      9,
		OutboxEnabled:     true,
	}
	if explicit.FetchFrequencyOrDefault() != 1500*time.Millisecond ||
		explicit.FetchSizeOrDefault() != 10 ||
		explicit.SentThreadCountOrDefault() != 2 ||
		explicit.OutboxDlqSizeOrDefault() != 8 ||
		explicit.OutboxDlqMaxRetryOrDefault() != 3 ||
		explicit.InboxDlqMaxRetryOrDefault() != 4 ||
		explicit.InboxDlqSizeOrDefault() != 9 {
		t.Fatalf("显式配置未生效: %+v", explicit)
	}

	rejects := []Config{
		{OutboxEnabled: true, FetchFrequencyMS: -1},
		{OutboxEnabled: true, FetchSize: -1},
		{OutboxEnabled: true, SentThreadCount: -1},
		{OutboxEnabled: true, OutboxDlqFrequencyMS: -1},
		{OutboxEnabled: true, OutboxDlqSize: -1},
		{OutboxEnabled: true, OutboxDlqMaxRetry: &retry, FetchFrequencyMS: 0},
		{OutboxEnabled: true, FetchTimeoutMinutes: -1},
		{OutboxEnabled: true, InboxDlqFrequencyMS: -1},
		{OutboxEnabled: true, InboxDlqSize: -1},
		{OutboxEnabled: true, OutboxRetentionDays: -1},
		{OutboxEnabled: true, InboxRetentionDays: -1},
		{OutboxEnabled: true, InboxDlqFrequencyMS: 1},
	}
	rejects[5].OutboxDlqMaxRetry = intPtr(-1)
	for i, cfg := range rejects {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("第 %d 组非法配置应失败: %+v", i, cfg)
		}
	}
}

func intPtr(v int) *int { return &v }

func TestParseMustAndComponentGuards(t *testing.T) {
	if _, ok := ParseAndValidate(nil, true); ok {
		t.Fatal("空消息应丢弃")
	}
	if _, ok := ParseAndValidate([]byte("{"), true); ok {
		t.Fatal("非法 JSON 应丢弃")
	}
	if _, ok := ParseAndValidate([]byte(`{"eventId":"e1"}`), true); ok {
		t.Fatal("空 topic 应丢弃")
	}

	func() {
		defer func() { _ = recover() }()
		MustNewAndStart(Deps{}, Config{OutboxEnabled: true, FetchFrequencyMS: -1})
		t.Fatal("非法配置应 panic")
	}()

	skipped := &Components{lockFactory: nopLockFactory{}, cfg: cfg.Runtime{InboxEnabled: true}}
	if err := skipped.Start(nil); err == nil {
		t.Fatal("缺少收件箱死信处理器应失败")
	}

	outboxComp := &Components{lockFactory: nopLockFactory{}, cfg: cfg.Runtime{OutboxEnabled: true}}
	if err := outboxComp.Start(context.Background()); err == nil {
		t.Fatal("缺少发件箱依赖应失败")
	}
	preset := &Components{
		lockFactory: nopLockFactory{},
		processor:   &outbox.Processor{},
		cfg:         cfg.Runtime{OutboxEnabled: true, FetchFrequency: time.Millisecond},
	}
	if err := preset.Start(context.Background()); err == nil {
		t.Fatal("过短的抓取频率应拒绝启动")
	}
	replay := &Components{
		lockFactory: nopLockFactory{},
		cfg:         cfg.Runtime{InboxEnabled: true},
		inboxDlqHandler: &InboxDlqHandler{
			dlqRepo:        &repo.InboxDlqRepo{},
			inboxRepo:      &repo.InboxRepo{},
			kafkaPublisher: edgePublisher{},
			cfg:            cfg.Runtime{InboxDlqFrequency: time.Millisecond},
		},
	}
	if err := replay.Start(nil); err == nil {
		t.Fatal("过短的收件箱死信频率应拒绝启动")
	}

	receiver := &Receiver{}
	receiver.resetInboxRetry(context.Background(), yomikafka.Message{})
	if _, err := redisInteger(7); err != nil {
		t.Fatal(err)
	}
	if _, err := redisInteger("bad"); err == nil {
		t.Fatal("非数值的重试计数应失败")
	}
	if err := receiver.moveToInboxDlq(context.Background(), Event{}, errors.New("x"), 1); err == nil {
		t.Fatal("缺少死信仓储应失败")
	}
}

func TestRetentionGuardsAndCleanErrors(t *testing.T) {
	ctx := context.Background()
	factory := nopLockFactory{}
	if err := NewRetentionCleaner(cfg.Runtime{
		OutboxCleanup: cfg.CleanupRuntime{Enabled: true},
	}, nil, nil).Start(ctx, factory); err == nil {
		t.Fatal("缺少发件箱仓储应失败")
	}
	if err := NewRetentionCleaner(cfg.Runtime{
		InboxCleanup: cfg.CleanupRuntime{Enabled: true},
	}, nil, nil).Start(ctx, factory); err == nil {
		t.Fatal("缺少收件箱仓储应失败")
	}
	if err := NewRetentionCleaner(cfg.Runtime{}, nil, nil).Start(ctx, factory); err != nil {
		t.Fatal(err)
	}
	if err := NewRetentionCleaner(cfg.Runtime{
		OutboxCleanup: cfg.CleanupRuntime{Enabled: true},
	}, &repo.OutboxRepo{}, nil).Start(ctx, factory); err == nil {
		t.Fatal("非法清理运行时配置应失败")
	}
	if err := NewRetentionCleaner(cfg.Runtime{
		InboxCleanup: cfg.CleanupRuntime{Enabled: true},
	}, nil, &repo.InboxRepo{}).Start(ctx, factory); err == nil {
		t.Fatal("非法收件箱清理配置应失败")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	validCleanup := cfg.CleanupRuntime{
		Enabled:    true,
		Retention:  24 * time.Hour,
		Frequency:  time.Second,
		BatchSize:  10,
		MaxBatches: 1,
	}
	if err := NewRetentionCleaner(cfg.Runtime{
		OutboxCleanup: validCleanup,
		InboxCleanup:  validCleanup,
	}, &repo.OutboxRepo{}, &repo.InboxRepo{}).Start(canceled, factory); err != nil {
		t.Fatal(err)
	}

	cleaner := &RetentionCleaner{}
	cleanup := cfg.CleanupRuntime{Retention: time.Hour, BatchSize: 10, MaxBatches: 2}
	if err := cleaner.clean(ctx, "outbox", cleanup,
		func(context.Context, time.Time, int) ([]int64, error) { return nil, nil },
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := cleaner.clean(ctx, "outbox", cleanup,
		func(context.Context, time.Time, int) ([]int64, error) { return []int64{1}, nil },
		func(context.Context, []int64) error { return errors.New("delete") },
	); err == nil {
		t.Fatal("删除失败应返回错误")
	}
}

// TestComponents_StartPartialFailures 验证后台任务中途失败时取消并返回错误
func TestComponents_StartPartialFailures(t *testing.T) {
	ctx := context.Background()
	comp := &Components{
		cfg: cfg.Runtime{
			ServiceName:    "svc",
			OutboxEnabled:  true,
			FetchFrequency: time.Second,
		},
		lockFactory: nopLockFactory{},
		processor:   &outbox.Processor{},
		dlqHandler:  outbox.NewDlqHandler(cfg.Runtime{ServiceName: "svc"}, nil, nil, nil, nil, nil),
	}
	if err := comp.Start(ctx); err == nil {
		t.Fatal("死信轮询频率非法时应启动失败")
	}

	retain := &Components{
		lockFactory: nopLockFactory{},
		retentionCleaner: NewRetentionCleaner(cfg.Runtime{
			ServiceName:   "svc",
			OutboxCleanup: cfg.CleanupRuntime{Enabled: true},
		}, nil, nil),
	}
	if err := retain.Start(ctx); err == nil {
		t.Fatal("清理仓储缺失时应启动失败")
	}
}
