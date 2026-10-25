// Redis Stream 消费辅助函数、ack 重试与发布边界的补充测试
//
// @author zwei
package redisstream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	redisv9 "github.com/redis/go-redis/v9"
)

// TestValueToString 验证字段值的三种转换分支
func TestValueToString(t *testing.T) {
	if got := valueToString("s"); got != "s" {
		t.Fatalf("string 分支不符: %q", got)
	}
	if got := valueToString([]byte("bytes")); got != "bytes" {
		t.Fatalf("[]byte 分支不符: %q", got)
	}
	if got := valueToString(42); got != "42" {
		t.Fatalf("default 分支不符: %q", got)
	}
	if got := stringifyValues(map[string]any{"a": "1", "b": []byte("2"), "c": 3})["c"]; got != "3" {
		t.Fatalf("stringifyValues 转换不符: %q", got)
	}
}

// TestIsUnrecoverableStreamError 验证 XACK 不可恢复错误判定
func TestIsUnrecoverableStreamError(t *testing.T) {
	if isUnrecoverableStreamError(nil) {
		t.Fatal("nil 错误不应判定为不可恢复")
	}
	if !isUnrecoverableStreamError(redisv9.ErrClosed) {
		t.Fatal("ErrClosed 应判定为不可恢复")
	}
	for _, msg := range []string{"NOGROUP no such group", "WRONGTYPE operation"} {
		if !isUnrecoverableStreamError(errors.New(msg)) {
			t.Fatalf("错误 %q 应判定为不可恢复", msg)
		}
	}
	if isUnrecoverableStreamError(errors.New("connection refused")) {
		t.Fatal("瞬时错误不应判定为不可恢复")
	}
}

// TestIsBusyGroup 验证消费组已存在错误判定
func TestIsBusyGroup(t *testing.T) {
	if isBusyGroup(nil) {
		t.Fatal("nil 错误不应判定为 BUSYGROUP")
	}
	if !isBusyGroup(errors.New("BUSYGROUP Consumer Group name already exists")) {
		t.Fatal("BUSYGROUP 应判定命中")
	}
	if isBusyGroup(errors.New("other error")) {
		t.Fatal("其他错误不应命中")
	}
}

// TestAckAndRetryDelays 验证 XACK 与读循环退避的抖动区间
// 抖动在封顶之后施加，名义上限允许 ±20% 溢出（与 event 包退避设计一致）
func TestAckAndRetryDelays(t *testing.T) {
	for i := 0; i < 50; i++ {
		ackLow := ackRetryMaxDelay * consumerRetryJitterMin / consumerRetryJitterDenom
		ackHigh := ackRetryMaxDelay * consumerRetryJitterMax / consumerRetryJitterDenom
		if got := nextAckRetryDelay(ackRetryMaxDelay); got < ackLow || got > ackHigh {
			t.Fatalf("ack 退避 %s 超出 [%s, %s]", got, ackLow, ackHigh)
		}
		low := consumerRetryMaxDelay * consumerRetryJitterMin / consumerRetryJitterDenom
		high := consumerRetryMaxDelay * consumerRetryJitterMax / consumerRetryJitterDenom
		if got := nextRetryDelay(consumerRetryMaxDelay); got < low || got > high {
			t.Fatalf("读循环退避 %s 超出 [%s, %s]", got, low, high)
		}
	}
}

// TestWaitForRetry 验证正常等待与上下文取消
func TestWaitForRetry(t *testing.T) {
	if err := waitForRetry(context.Background(), 10*time.Millisecond); err != nil {
		t.Fatalf("正常等待不应报错: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForRetry(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消上下文应立即返回 Canceled，实际 %v", err)
	}
}

// TestEntryPayload 验证载荷提取的空值与命中分支
func TestEntryPayload(t *testing.T) {
	var nilEntry Entry
	if nilEntry.Payload() != nil {
		t.Fatal("nil Values 应返回 nil")
	}
	if got := (Entry{Values: map[string]string{"other": "x"}}).Payload(); got != nil {
		t.Fatalf("缺省字段应返回 nil，实际 %v", got)
	}
	if string((Entry{Values: map[string]string{PayloadField: "abc"}}).Payload()) != "abc" {
		t.Fatal("命中字段应返回内容")
	}
}

// TestInvokeHandler 验证 handler 正常返回与 panic 恢复
func TestInvokeHandler(t *testing.T) {
	entry := Entry{Stream: "s", ID: "1-1"}
	if got := invokeHandler(context.Background(), func(context.Context, Entry) HandleResult {
		return HandleAck
	}, entry); got != HandleAck {
		t.Fatalf("正常返回不符: %v", got)
	}
	if got := invokeHandler(context.Background(), func(context.Context, Entry) HandleResult {
		panic("handler boom")
	}, entry); got != HandleRetry {
		t.Fatalf("panic 应按 HandleRetry 处理: %v", got)
	}
}

// streamLen 返回 stream 的当前消息数
func streamLen(t *testing.T, client *redisv9.Client, key string) int64 {
	t.Helper()
	n, err := client.XLen(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("XLen %s 失败: %v", key, err)
	}
	return n
}

// newTestConsumer 构建指向内存 Redis 的 consumer 实例
func newTestConsumer(t *testing.T) (*consumer, *redisv9.Client, *miniredis.Miniredis, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	return &consumer{
		client: client,
		cfg: resolvedGroupConfig{
			logicalStream: "s",
			streamKey:     "s",
			group:         "g",
			consumer:      "c1",
			deadStreamKey: "s_dead",
		},
	}, client, mr, func() { _ = client.Close() }
}

// TestFinalizeHandleResult 验证各 HandleResult 的收尾行为
func TestFinalizeHandleResult(t *testing.T) {
	c, client, mr, cleanup := newTestConsumer(t)
	defer cleanup()
	ctx := context.Background()
	entry := Entry{Stream: "s", ID: "1-1", Values: map[string]string{PayloadField: "v"}}
	_ = client

	shouldAck, err := c.finalizeHandleResult(ctx, entry, HandleAck)
	if err != nil || !shouldAck {
		t.Fatalf("HandleAck 应纳入确认: %v %v", shouldAck, err)
	}

	shouldAck, err = c.finalizeHandleResult(ctx, entry, HandleRetry)
	if err != nil || shouldAck {
		t.Fatalf("HandleRetry 应保留 PEL: %v %v", shouldAck, err)
	}

	shouldAck, err = c.finalizeHandleResult(ctx, entry, HandleResult(99))
	if err != nil || shouldAck {
		t.Fatalf("未知结果应按 Retry 保留 PEL: %v %v", shouldAck, err)
	}

	shouldAck, err = c.finalizeHandleResult(ctx, entry, HandleDead)
	if err != nil || !shouldAck {
		t.Fatalf("HandleDead 应写入死信并确认: %v %v", shouldAck, err)
	}
	if n := streamLen(t, client, "s_dead"); n != 1 {
		t.Fatalf("死信 stream 应有 1 条消息，实际 %d", n)
	}

	// 服务不可用时 HandleDead 写入失败：保留 PEL 且不报错
	mr.Close()
	shouldAck, err = c.finalizeHandleResult(ctx, entry, HandleDead)
	if err != nil || shouldAck {
		t.Fatalf("死信写入失败应保留 PEL: %v %v", shouldAck, err)
	}
}

// TestAckWithRetry_Branches 验证 XACK 的不可恢复、取消与重试耗尽分支
func TestAckWithRetry_Branches(t *testing.T) {
	c, _, mr, cleanup := newTestConsumer(t)
	defer cleanup()
	ctx := context.Background()

	// 未创建消费组时 XACK 返回 NOGROUP → 不可恢复 → 立即放弃返回 nil
	if err := c.ackWithRetry(ctx, "1-1"); err != nil {
		t.Fatalf("NOGROUP 应立即放弃返回 nil: %v", err)
	}

	// 上下文取消应原样返回
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := c.ackWithRetry(canceled, "1-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消上下文应返回 Canceled，实际 %v", err)
	}

	// 服务不可用时重试耗尽后放弃（瞬时错误重试 ackRetryMaxAttempts 次）
	mr.Close()
	start := time.Now()
	if err := c.ackWithRetry(ctx, "1-1"); err != nil {
		t.Fatalf("重试耗尽应返回 nil: %v", err)
	}
	if elapsed := time.Since(start); elapsed < ackRetryInitialDelay {
		t.Fatalf("应经历多轮退避重试，实际耗时 %v", elapsed)
	}
}

// TestPublisherAsyncMethodsSyncFallback 验证同步发布器的 Async 方法退化为同步发送
func TestPublisherAsyncMethodsSyncFallback(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	pub := NewPublisher(client, RuntimeConfig{})
	ctx := context.Background()

	if err := pub.AddAsync(ctx, "s", map[string]string{PayloadField: "v1"}); err != nil {
		t.Fatalf("AddAsync 失败: %v", err)
	}
	if err := pub.AddBytesAsync(ctx, "s", []byte("v2")); err != nil {
		t.Fatalf("AddBytesAsync 失败: %v", err)
	}
	if err := pub.AddJSONAsync(ctx, "s", map[string]string{"k": "v3"}); err != nil {
		t.Fatalf("AddJSONAsync 失败: %v", err)
	}
	if n := streamLen(t, client, "s"); n != 3 {
		t.Fatalf("期望 3 条消息，实际 %d 条", n)
	}

	// 关闭后各入口均拒绝
	if err := pub.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	if _, err := pub.Add(ctx, "s", map[string]string{PayloadField: "v"}); err == nil {
		t.Fatal("关闭后 Add 应返回错误")
	}
	if err := pub.AddAsync(ctx, "s", map[string]string{PayloadField: "v"}); err == nil {
		t.Fatal("关闭后 AddAsync 应返回错误")
	}
	if err := pub.AddBytesAsync(ctx, "s", []byte("v")); err == nil {
		t.Fatal("关闭后 AddBytesAsync 应返回错误")
	}
	if err := pub.AddJSONAsync(ctx, "s", "v"); err == nil {
		t.Fatal("关闭后 AddJSONAsync 应返回错误")
	}
}

// TestPublisherInputGuards 验证发布入参校验
func TestPublisherInputGuards(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	pub := NewPublisher(client, RuntimeConfig{})
	ctx := context.Background()

	if _, err := pub.Add(ctx, "  ", map[string]string{PayloadField: "v"}); err == nil {
		t.Fatal("空 stream 应返回错误")
	}
	if _, err := pub.Add(ctx, "s", nil); err == nil {
		t.Fatal("空 values 应返回错误")
	}
	if _, err := pub.AddBytes(ctx, "s", nil); err == nil {
		t.Fatal("nil payload 应返回错误")
	}
	if _, err := pub.AddJSON(ctx, "s", nil); err == nil {
		t.Fatal("nil value 应返回错误")
	}
	if _, err := pub.AddJSON(ctx, "s", map[string]any{"ch": make(chan int)}); err == nil {
		t.Fatal("不可序列化对象应返回错误")
	}

	// KeyPrefix 拼接
	prefixed := NewPublisher(client, RuntimeConfig{KeyPrefix: "rs:", DefaultMaxLen: 100})
	if _, err := prefixed.Add(ctx, "orders", map[string]string{PayloadField: "v"}); err != nil {
		t.Fatalf("带前缀发布失败: %v", err)
	}
	if n := streamLen(t, client, "rs:orders"); n != 1 {
		t.Fatalf("带前缀的 stream 应有 1 条消息，实际 %d 条", n)
	}
}

// TestAsyncPublisherDelegationAndGuards 验证异步发布器的委派、守卫与无工作池降级
func TestAsyncPublisherDelegationAndGuards(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	syncPub := NewPublisher(client, RuntimeConfig{})
	// 无工作池：sendTimeout<=0 取默认值，Async 方法直接同步发送
	async := NewAsyncPublisher(syncPub, nil, 0)
	ctx := context.Background()

	if err := async.AddAsync(ctx, "s", map[string]string{PayloadField: "v1"}); err != nil {
		t.Fatalf("无工作池 AddAsync 应同步发送: %v", err)
	}
	if err := async.AddBytesAsync(ctx, "s", []byte("v2")); err != nil {
		t.Fatalf("无工作池 AddBytesAsync 应同步发送: %v", err)
	}
	if err := async.AddJSONAsync(ctx, "s", map[string]string{"k": "v3"}); err != nil {
		t.Fatalf("无工作池 AddJSONAsync 应同步发送: %v", err)
	}
	if _, err := async.Add(ctx, "s", map[string]string{PayloadField: "v4"}); err != nil {
		t.Fatalf("Add 委派失败: %v", err)
	}
	if _, err := async.AddJSON(ctx, "s", map[string]string{"k": "v5"}); err != nil {
		t.Fatalf("AddJSON 委派失败: %v", err)
	}
	if n := streamLen(t, client, "s"); n != 5 {
		t.Fatalf("期望 5 条消息，实际 %d 条", n)
	}

	// 入参守卫（入队前校验）
	if err := async.AddAsync(ctx, "  ", map[string]string{PayloadField: "v"}); err == nil {
		t.Fatal("空 stream 应返回错误")
	}
	if err := async.AddAsync(ctx, "s", nil); err == nil {
		t.Fatal("空 values 应返回错误")
	}
	if err := async.AddBytesAsync(ctx, "s", nil); err == nil {
		t.Fatal("nil payload 应返回错误")
	}
	if err := async.AddJSONAsync(ctx, "s", nil); err == nil {
		t.Fatal("nil value 应返回错误")
	}
	if err := async.AddJSONAsync(ctx, "s", map[string]any{"ch": make(chan int)}); err == nil {
		t.Fatal("不可序列化对象应返回错误")
	}

	// Close 排空工作池并释放底层发布者（无工作池时仅释放 sync）
	if err := async.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
}

// TestMustNewComponents 验证 Fail Fast 构造的成功与失败路径
func TestMustNewComponents(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	components := MustNewComponents(Config{PublishWorker: PublishWorkerConfig{WorkNum: 1, Size: 8}}, client)
	if components == nil || components.Publisher == nil || components.Consumer == nil {
		t.Fatal("MustNewComponents 应返回完整组件")
	}
	if _, err := components.Publisher.AddJSON(context.Background(), "s", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("组件应可用: %v", err)
	}
	if err := components.Close(); err != nil {
		t.Fatalf("关闭组件失败: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("nil client 应 panic")
		}
	}()
	MustNewComponents(Config{}, nil)
}

// TestNewComponentsNilClient 验证装配阶段对 nil client 的校验
func TestNewComponentsNilClient(t *testing.T) {
	if _, err := NewComponents(Config{}, nil); err == nil {
		t.Fatal("nil client 应返回错误")
	}
}

// TestComponentsCloseNilSafe 验证 nil 组件的关闭安全
func TestComponentsCloseNilSafe(t *testing.T) {
	var c *Components
	if err := c.Close(); err != nil {
		t.Fatalf("nil 组件关闭应返回 nil: %v", err)
	}
}

// TestBuildPublishWorkerConfigDefaults 验证工作池配置默认值与透传
func TestBuildPublishWorkerConfigDefaults(t *testing.T) {
	def := buildPublishWorkerConfig(PublishWorkerConfig{})
	if def.WorkNum != defaultPublishWorkerNum || def.Size != defaultPublishWorkerSize {
		t.Fatalf("默认工作池配置期望 %d/%d，实际 %d/%d", defaultPublishWorkerNum, defaultPublishWorkerSize, def.WorkNum, def.Size)
	}

	custom := buildPublishWorkerConfig(PublishWorkerConfig{WorkNum: 2, Size: 16, SubmitTimeoutSeconds: 5})
	if custom.WorkNum != 2 || custom.Size != 16 || custom.SubmitTimeout != 5*time.Second {
		t.Fatalf("自定义工作池配置不符: %+v", custom)
	}
}

// TestConfigValidatePrefixWarningBranch 验证前缀与负值校验分支
func TestConfigValidatePrefixWarningBranch(t *testing.T) {
	negative := int64(-1)
	if err := (Config{DefaultMaxLen: &negative}).Validate(); err == nil {
		t.Fatal("default_maxlen 为负应返回错误")
	}
	// 无冒号前缀仅告警不报错
	if err := (Config{KeyPrefix: "rs"}).Validate(); err != nil {
		t.Fatalf("无冒号前缀不应报错: %v", err)
	}
}
