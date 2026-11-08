// Package event 的收件箱死信与消费重试测试
//
// @author zwei
package event

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hwwadmin/yomi-boot/component/common/db/dbtest"
	"github.com/hwwadmin/yomi-boot/component/common/db/tx"
	"github.com/hwwadmin/yomi-boot/component/common/distlock"
	"github.com/hwwadmin/yomi-boot/component/common/event/cfg"
	"github.com/hwwadmin/yomi-boot/component/common/event/model"
	"github.com/hwwadmin/yomi-boot/component/common/event/repo"
	yomiredis "github.com/hwwadmin/yomi-boot/component/common/redis"
	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"
	"github.com/hwwadmin/yomi-boot/component/common/snowflake"

	yomikafka "github.com/hwwadmin/yomi-boot/component/common/kafka"
)

// TestReceiverShouldMoveToInboxDlq 验证死信转存阈值与无限重试配置
func TestReceiverShouldMoveToInboxDlq(t *testing.T) {
	dlqRepo := &repo.InboxDlqRepo{}
	cases := []struct {
		name       string
		repo       *repo.InboxDlqRepo
		maxRetry   int
		retryCount int
		want       bool
	}{
		{name: "below threshold", repo: dlqRepo, maxRetry: 2, retryCount: 0, want: false},
		{name: "last retry", repo: dlqRepo, maxRetry: 2, retryCount: 1, want: false},
		{name: "at threshold", repo: dlqRepo, maxRetry: 2, retryCount: 2, want: true},
		{name: "above threshold", repo: dlqRepo, maxRetry: 2, retryCount: 3, want: true},
		{name: "zero retry", repo: dlqRepo, maxRetry: 0, retryCount: 0, want: true},
		{name: "infinite retry", repo: dlqRepo, maxRetry: -1, retryCount: 100, want: false},
		{name: "no dlq repo", repo: nil, maxRetry: 2, retryCount: 100, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Receiver{
				inboxDlqRepo: tc.repo,
				cfg:          cfg.Runtime{InboxDlqMaxRetry: tc.maxRetry},
			}
			if got := r.shouldMoveToInboxDlq(tc.retryCount); got != tc.want {
				t.Fatalf("shouldMoveToInboxDlq 期望 %v，实际 %v", tc.want, got)
			}
		})
	}
}

// TestInboxRetryCountInRedis 验证失败次数按 Kafka 位点累加，重置后从零开始，并带有过期时间
func TestInboxRetryCountInRedis(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	receiver := &Receiver{
		redis: yomiredis.NewRedisHelper(client),
		cfg:   cfg.Runtime{ServiceName: "svc"},
	}
	msg := yomikafka.Message{Topic: "topic", Partition: 1, Offset: 10}
	key := receiver.cfg.RetryCountKey(msg.Topic, msg.Partition, msg.Offset)
	if key != "event:receiver:retry:svc:topic:1:10" {
		t.Fatalf("重试计数键不符: %s", key)
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		got, err := receiver.incrementInboxRetry(ctx, msg)
		if err != nil {
			t.Fatalf("第 %d 次累加失败: %v", i+1, err)
		}
		if got != i {
			t.Fatalf("第 %d 次累加期望重试次数 %d，实际 %d", i+1, i, got)
		}
	}

	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("读取重试计数 TTL 失败: %v", err)
	}
	if ttl <= 6*24*time.Hour || ttl > inboxRetryCountTTL {
		t.Fatalf("重试计数 TTL 应接近 7 天，实际 %s", ttl)
	}

	receiver.resetInboxRetry(ctx, msg)
	exists, err := client.Exists(ctx, key).Result()
	if err != nil {
		t.Fatalf("检查重试计数键失败: %v", err)
	}
	if exists != 0 {
		t.Fatal("重置后重试计数键应被删除")
	}
	got, err := receiver.incrementInboxRetry(ctx, msg)
	if err != nil {
		t.Fatalf("重置后累加失败: %v", err)
	}
	if got != 0 {
		t.Fatalf("重置后第一次累加期望 0，实际 %d", got)
	}

	if _, err := (&Receiver{cfg: receiver.cfg}).incrementInboxRetry(ctx, msg); err == nil {
		t.Fatal("Redis 未初始化时累加应返回错误")
	}
}

// TestTruncateInboxDlqError 验证死信错误信息按字符截断且不破坏 UTF-8
func TestTruncateInboxDlqError(t *testing.T) {
	if got := truncateInboxDlqError(nil); got != "" {
		t.Fatalf("nil error 期望空串，实际 %q", got)
	}
	short := fmt.Errorf("boom")
	if got := truncateInboxDlqError(short); got != "boom" {
		t.Fatalf("短错误期望原样返回，实际 %q", got)
	}

	long := fmt.Errorf("%s", strings.Repeat("错", inboxDlqErrorMsgLimit+10))
	got := truncateInboxDlqError(long)
	if !utf8.ValidString(got) {
		t.Fatal("截断后的错误信息不是合法 UTF-8")
	}
	if !strings.HasSuffix(got, "...[truncated]") {
		t.Fatalf("截断后的错误信息缺少截断标记: %q", got)
	}
	if utf8.RuneCountInString(got) != inboxDlqErrorMsgLimit+len("...[truncated]") {
		t.Fatalf("截断长度不符，实际 %d", utf8.RuneCountInString(got))
	}
}

// TestConfigValidateInboxDlqMaxRetry 验证消费最大重试配置的合法域
func TestConfigValidateInboxDlqMaxRetry(t *testing.T) {
	cases := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{name: "infinite", value: -1},
		{name: "no retry", value: 0},
		{name: "finite", value: 16},
		{name: "invalid", value: -2, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := tc.value
			config := Config{InboxEnabled: true, InboxDlqMaxRetry: &value}
			err := config.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("期望配置校验失败")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望配置校验通过，实际 %v", err)
			}
		})
	}
}

// TestInvokeEventHandlerRecoversPanic 验证业务 panic 被转为普通错误，便于进入重试计数
func TestInvokeEventHandlerRecoversPanic(t *testing.T) {
	err := invokeEventHandler(func(context.Context, Event) error {
		panic("boom")
	}, context.Background(), Event{})
	if err == nil {
		t.Fatal("业务 panic 应转为错误")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("错误信息应包含 panic 内容，实际 %q", err.Error())
	}
}

// TestReceiverMovesToInboxDlqAfterRetryExhausted 验证业务连续失败后写入死信并 ack
func TestReceiverMovesToInboxDlqAfterRetryExhausted(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}),
	)
	defer cleanup()

	client, redisCleanup := redistest.NewClient(t)
	defer redisCleanup()

	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("创建分布式锁工厂失败: %v", err)
	}
	gen, err := snowflake.NewGenerator(1)
	if err != nil {
		t.Fatalf("创建雪花 ID 生成器失败: %v", err)
	}

	receiver := NewReceiver(
		repo.NewInboxRepo(gormDB),
		repo.NewInboxDlqRepo(gormDB),
		tx.NewLocalRunner(gormDB),
		factory,
		yomiredis.NewRedisHelper(client),
		cfg.Runtime{ServiceName: "svc", InboxDlqMaxRetry: 2},
		gen,
	)
	handler := receiver.WrapHandler(func(context.Context, Event) error {
		return fmt.Errorf("business failed")
	})
	msg := yomikafka.Message{
		Topic:     "topic",
		Partition: 0,
		Offset:    1,
		Value:     []byte(`{"eventId":"event-1","topic":"topic","payload":"payload-1"}`),
	}
	ctx := context.Background()

	for attempt := 1; attempt <= 2; attempt++ {
		if result := handler(ctx, msg); result != yomikafka.Retry() {
			t.Fatalf("第 %d 次失败应重试，实际 %s", attempt, result)
		}
	}
	if result := handler(ctx, msg); result != yomikafka.AckOK() {
		t.Fatalf("达到最大重试次数后应提交并转入死信，实际 %s", result)
	}

	var dlqRows []model.SysEventInboxDlq
	if err := gormDB.Find(&dlqRows).Error; err != nil {
		t.Fatalf("查询收件箱死信失败: %v", err)
	}
	if len(dlqRows) != 1 {
		t.Fatalf("期望 1 条死信，实际 %d 条", len(dlqRows))
	}
	if dlqRows[0].EventID != "event-1" || dlqRows[0].RetryCount != 2 {
		t.Fatalf("死信内容不符: %+v", dlqRows[0])
	}
	if !strings.Contains(dlqRows[0].ErrorMsg, "business failed") {
		t.Fatalf("死信错误信息不符: %q", dlqRows[0].ErrorMsg)
	}

	var inboxCount int64
	if err := gormDB.Model(&model.SysEventInbox{}).Count(&inboxCount).Error; err != nil {
		t.Fatalf("查询收件箱失败: %v", err)
	}
	if inboxCount != 0 {
		t.Fatalf("业务事务应回滚，inbox 期望 0 条，实际 %d 条", inboxCount)
	}
	exists, err := client.Exists(ctx, "event:receiver:retry:svc:topic:0:1").Result()
	if err != nil {
		t.Fatalf("检查重试计数键失败: %v", err)
	}
	if exists != 0 {
		t.Fatal("转入死信后应清除重试计数")
	}
}

// TestReceiverRetryCountSharedAcrossReceivers 验证分区换到另一个接收器后失败次数继续累加
func TestReceiverRetryCountSharedAcrossReceivers(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}),
	)
	defer cleanup()

	client, redisCleanup := redistest.NewClient(t)
	defer redisCleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("创建分布式锁工厂失败: %v", err)
	}
	redisSvc := yomiredis.NewRedisHelper(client)
	runtime := cfg.Runtime{ServiceName: "svc", InboxDlqMaxRetry: 2}
	newReceiver := func() *Receiver {
		t.Helper()
		gen, genErr := snowflake.NewGenerator(1)
		if genErr != nil {
			t.Fatalf("创建雪花 ID 生成器失败: %v", genErr)
		}
		return NewReceiver(
			repo.NewInboxRepo(gormDB),
			repo.NewInboxDlqRepo(gormDB),
			tx.NewLocalRunner(gormDB),
			factory,
			redisSvc,
			runtime,
			gen,
		)
	}

	fail := func(context.Context, Event) error { return fmt.Errorf("business failed") }
	msg := yomikafka.Message{
		Topic:     "topic",
		Partition: 0,
		Offset:    7,
		Value:     []byte(`{"eventId":"event-shared","topic":"topic","payload":"payload-1"}`),
	}
	ctx := context.Background()

	if result := newReceiver().WrapHandler(fail)(ctx, msg); result != yomikafka.Retry() {
		t.Fatalf("第一个接收器的首次失败应重试，实际 %s", result)
	}
	if result := newReceiver().WrapHandler(fail)(ctx, msg); result != yomikafka.Retry() {
		t.Fatalf("第二个接收器接手前应重试，实际 %s", result)
	}
	if result := newReceiver().WrapHandler(fail)(ctx, msg); result != yomikafka.AckOK() {
		t.Fatalf("跨接收器累加到上限后应提交并转入死信，实际 %s", result)
	}

	var count int64
	if err := gormDB.Model(&model.SysEventInboxDlq{}).Count(&count).Error; err != nil {
		t.Fatalf("查询收件箱死信失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("期望 1 条死信，实际 %d 条", count)
	}
}

// TestReceiverRetryCountRedisFailureStaysRetry 验证计数写入失败时不进死信、不提交
func TestReceiverRetryCountRedisFailureStaysRetry(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}),
	)
	defer cleanup()

	live, liveCleanup := redistest.NewClient(t)
	defer liveCleanup()
	factory, err := distlock.NewFactory(live)
	if err != nil {
		t.Fatalf("创建分布式锁工厂失败: %v", err)
	}
	dead, deadCleanup := redistest.NewClient(t)
	deadCleanup()

	gen, err := snowflake.NewGenerator(1)
	if err != nil {
		t.Fatalf("创建雪花 ID 生成器失败: %v", err)
	}
	receiver := NewReceiver(
		repo.NewInboxRepo(gormDB),
		repo.NewInboxDlqRepo(gormDB),
		tx.NewLocalRunner(gormDB),
		factory,
		yomiredis.NewRedisHelper(dead),
		cfg.Runtime{ServiceName: "svc", InboxDlqMaxRetry: 0},
		gen,
	)
	handler := receiver.WrapHandler(func(context.Context, Event) error {
		return fmt.Errorf("business failed")
	})
	msg := yomikafka.Message{
		Topic:     "topic",
		Partition: 0,
		Offset:    8,
		Value:     []byte(`{"eventId":"event-redis-down","topic":"topic","payload":"payload-1"}`),
	}
	if result := handler(context.Background(), msg); result != yomikafka.Retry() {
		t.Fatalf("Redis 计数失败时应重试，实际 %s", result)
	}

	var count int64
	if err := gormDB.Model(&model.SysEventInboxDlq{}).Count(&count).Error; err != nil {
		t.Fatalf("查询收件箱死信失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("Redis 计数失败时不应写死信，实际 %d 条", count)
	}
}

// TestCanceledFailureDoesNotConsumeRetryBudget 验证停机或重平衡造成的取消不消耗重试额度
func TestCanceledFailureDoesNotConsumeRetryBudget(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}),
	)
	defer cleanup()

	client, redisCleanup := redistest.NewClient(t)
	defer redisCleanup()
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("创建分布式锁工厂失败: %v", err)
	}
	gen, err := snowflake.NewGenerator(1)
	if err != nil {
		t.Fatalf("创建雪花 ID 生成器失败: %v", err)
	}
	receiver := NewReceiver(
		repo.NewInboxRepo(gormDB),
		repo.NewInboxDlqRepo(gormDB),
		tx.NewLocalRunner(gormDB),
		factory,
		yomiredis.NewRedisHelper(client),
		cfg.Runtime{ServiceName: "svc", InboxDlqMaxRetry: 1},
		gen,
	)
	canceled := true
	handler := receiver.WrapHandler(func(context.Context, Event) error {
		if canceled {
			return context.Canceled
		}
		return fmt.Errorf("business failed")
	})
	msg := yomikafka.Message{
		Topic:     "topic",
		Partition: 0,
		Offset:    9,
		Value:     []byte(`{"eventId":"event-cancel","topic":"topic","payload":"payload-1"}`),
	}
	ctx := context.Background()
	if result := handler(ctx, msg); result != yomikafka.Retry() {
		t.Fatalf("上下文取消应重试，实际 %s", result)
	}
	exists, err := client.Exists(ctx, "event:receiver:retry:svc:topic:0:9").Result()
	if err != nil {
		t.Fatalf("检查重试计数键失败: %v", err)
	}
	if exists != 0 {
		t.Fatal("上下文取消不应写入重试计数")
	}

	canceled = false
	if result := handler(ctx, msg); result != yomikafka.Retry() {
		t.Fatalf("取消之后的首次业务失败应重试，实际 %s", result)
	}
	if result := handler(ctx, msg); result != yomikafka.AckOK() {
		t.Fatalf("第二次业务失败应达到上限并提交，实际 %s", result)
	}
}

// scriptRedis 只预设脚本求值与删除结果的 Redis 桩
type scriptRedis struct {
	result  any
	evalErr error
	delErr  error
}

func (scriptRedis) OpsStr4Set(context.Context, string, string, time.Duration) error { return nil }
func (scriptRedis) OpsStr4SetNoTTL(context.Context, string, string) error           { return nil }
func (scriptRedis) OpsStr4SetJSON(context.Context, string, any, time.Duration) error {
	return nil
}
func (scriptRedis) OpsStr4SetJSONNoTTL(context.Context, string, any) error { return nil }
func (scriptRedis) OpsStr4Get(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (scriptRedis) OpsStr4GetJSON(context.Context, string, any) (bool, error) { return false, nil }
func (scriptRedis) OpsHash4Set(context.Context, string, map[string]any, time.Duration) error {
	return nil
}
func (scriptRedis) OpsHash4Get(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}
func (scriptRedis) OpsHash4GetAll(context.Context, string) (map[string]string, bool, error) {
	return nil, false, nil
}
func (s scriptRedis) OpsKey4Delete(context.Context, ...string) error { return s.delErr }
func (scriptRedis) OpsKey4Exists(context.Context, string) (bool, error) {
	return false, nil
}
func (scriptRedis) OpsKey4Expire(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}
func (scriptRedis) OpsSet4Add(context.Context, string, ...string) error    { return nil }
func (scriptRedis) OpsSet4Remove(context.Context, string, ...string) error { return nil }
func (s scriptRedis) OpsScript4Eval(context.Context, string, []string, ...any) (any, error) {
	return s.result, s.evalErr
}
func (scriptRedis) OpsRateLimit4Allow(context.Context, string, int64, int64, time.Duration) (bool, error) {
	return false, nil
}
func (scriptRedis) OpsSet4Members(context.Context, string) ([]string, error) { return nil, nil }

type failIDGen struct{}

func (failIDGen) NextID() (int64, error) { return 0, errors.New("id") }

// TestReceiver_RetryCounterAndDlqID 验证计数类型异常、删除失败和 ID 生成失败
func TestReceiver_RetryCounterAndDlqID(t *testing.T) {
	receiver := &Receiver{
		cfg:          cfg.Runtime{ServiceName: "svc"},
		redis:        scriptRedis{result: "nope", delErr: errors.New("del")},
		inboxDlqRepo: &repo.InboxDlqRepo{},
		idGen:        failIDGen{},
	}
	msg := yomikafka.Message{Topic: "topic", Partition: 1, Offset: 2}
	if _, err := receiver.incrementInboxRetry(context.Background(), msg); err == nil {
		t.Fatal("异常计数类型应返回错误")
	}
	receiver.resetInboxRetry(context.Background(), msg)
	if err := receiver.moveToInboxDlq(context.Background(), Event{EventID: "event-1"}, errors.New("boom"), 2); err == nil {
		t.Fatal("ID 生成失败应返回错误")
	}
}

func TestReceiver_GuardBranches(t *testing.T) {
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
