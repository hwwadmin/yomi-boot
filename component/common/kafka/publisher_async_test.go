package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/worker"
)

// submitTimeoutWorker 模拟工作池队列持续满员：入队等待直到超时
type submitTimeoutWorker struct{}

// Start 实现 Start 接口（测试桩）
func (w *submitTimeoutWorker) Start() {}

// Submit 实现 Submit 接口（测试桩，模拟入队等待超时）
func (w *submitTimeoutWorker) Submit(_ worker.WorkerHandler) error {
	return errors.New("worker.kafka.publish.submit_timeout")
}

// SubmitContext 实现 SubmitContext 接口（测试桩，模拟入队等待超时）
func (w *submitTimeoutWorker) SubmitContext(_ context.Context, _ worker.WorkerHandler) error {
	return errors.New("worker.kafka.publish.submit_timeout")
}

// TrySubmit 实现 TrySubmit 接口（测试桩，模拟入队等待超时）
func (w *submitTimeoutWorker) TrySubmit(_ worker.WorkerHandler) error {
	return errors.New("worker.kafka.publish.submit_timeout")
}

// Pending 实现 Pending 接口（测试桩）
func (w *submitTimeoutWorker) Pending() int { return 0 }

// Close 实现 Close 接口（测试桩）
func (w *submitTimeoutWorker) Close() error { return nil }

// Wait 实现 Wait 接口（测试桩）
func (w *submitTimeoutWorker) Wait() {}

// enqueuedWorker 模拟入队成功且任务尚未执行
type enqueuedWorker struct{}

// Start 实现 Start 接口（测试桩）
func (w *enqueuedWorker) Start() {}

// Submit 实现 Submit 接口（测试桩，模拟入队成功）
func (w *enqueuedWorker) Submit(_ worker.WorkerHandler) error { return nil }

// SubmitContext 实现 SubmitContext 接口（测试桩，模拟入队成功）
func (w *enqueuedWorker) SubmitContext(_ context.Context, _ worker.WorkerHandler) error {
	return nil
}

// TrySubmit 实现 TrySubmit 接口（测试桩，模拟入队成功）
func (w *enqueuedWorker) TrySubmit(_ worker.WorkerHandler) error { return nil }

// Pending 实现 Pending 接口（测试桩）
func (w *enqueuedWorker) Pending() int { return 0 }

// Close 实现 Close 接口（测试桩）
func (w *enqueuedWorker) Close() error { return nil }

// Wait 实现 Wait 接口（测试桩）
func (w *enqueuedWorker) Wait() {}

// callerCtxWorker 模拟入队阶段调用方 ctx 已取消（复用 submitTimeoutWorker 其余接口方法）
type callerCtxWorker struct {
	submitTimeoutWorker
}

// SubmitContext 实现 SubmitContext 接口（测试桩，原样返回传入 ctx 的错误）
func (w *callerCtxWorker) SubmitContext(ctx context.Context, _ worker.WorkerHandler) error {
	return ctx.Err()
}

// recordingPublisher 记录同步 Publish 是否被调用
type recordingPublisher struct {
	publishCalls int
	publishErr   error
}

// Publish 实现 Publish 接口（测试桩，记录调用次数）
func (p *recordingPublisher) Publish(_ context.Context, _ PublishRequest) error {
	p.publishCalls++
	return p.publishErr
}

// PublishBatch 实现 PublishBatch 接口（测试桩）
func (p *recordingPublisher) PublishBatch(_ context.Context, _ []PublishRequest) error { return nil }

// PublishJSON 实现 PublishJSON 接口（测试桩）
func (p *recordingPublisher) PublishJSON(_ context.Context, _ string, _ string, _ any) error {
	return nil
}

// PublishAsync 实现 PublishAsync 接口（测试桩）
func (p *recordingPublisher) PublishAsync(_ context.Context, _ PublishRequest) error { return nil }

// PublishBatchAsync 实现 PublishBatchAsync 接口（测试桩）
func (p *recordingPublisher) PublishBatchAsync(_ context.Context, _ []PublishRequest) error {
	return nil
}

// PublishJSONAsync 实现 PublishJSONAsync 接口（测试桩）
func (p *recordingPublisher) PublishJSONAsync(_ context.Context, _ string, _ string, _ any) error {
	return nil
}

// Close 实现 Close 接口（测试桩）
func (p *recordingPublisher) Close() error { return nil }

// TestShouldFallbackSync 验证工作池错误是否应触发同步降级
func TestShouldFallbackSync(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("worker.kafka.publish.queue_full"), true},
		{errors.New("worker.kafka.publish.not_run"), true},
		{errors.New("worker.kafka.publish.stopped"), true},
		{errors.New("worker.kafka.publish.submit_timeout"), true},
		{errors.New("other"), false},
		{context.Canceled, false},
	}
	for _, tc := range cases {
		if got := shouldFallbackSync(tc.err); got != tc.want {
			t.Fatalf("shouldFallbackSync(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// TestPublishAsync_FallbackToSyncWhenSubmitWaitTimeout 验证入队等待超时后降级调用同步 Publish
func TestPublishAsync_FallbackToSyncWhenSubmitWaitTimeout(t *testing.T) {
	recorder := &recordingPublisher{}
	asyncPub := newAsyncPublisher(recorder, &submitTimeoutWorker{}, time.Second)

	ctx := context.Background()
	if err := asyncPub.PublishAsync(ctx, PublishRequest{
		Topic: "topic",
		Value: []byte("payload"),
	}); err != nil {
		t.Fatalf("PublishAsync: %v", err)
	}
	if recorder.publishCalls != 1 {
		t.Fatalf("sync publish calls = %d, want 1", recorder.publishCalls)
	}
}

// TestPublishAsync_EnqueuedWithoutSyncFallback 验证入队成功时直接返回、不触发同步发送
func TestPublishAsync_EnqueuedWithoutSyncFallback(t *testing.T) {
	recorder := &recordingPublisher{}
	asyncPub := newAsyncPublisher(recorder, &enqueuedWorker{}, time.Second)

	if err := asyncPub.PublishAsync(context.Background(), PublishRequest{
		Topic: "topic",
		Value: []byte("payload"),
	}); err != nil {
		t.Fatalf("PublishAsync: %v", err)
	}
	if recorder.publishCalls != 0 {
		t.Fatalf("sync publish calls = %d, want 0", recorder.publishCalls)
	}
}

// TestPublishAsync_ReturnsErrorWhenCallerCtxCanceled 验证调用方 ctx 取消时入队失败原样返回错误、不降级同步
func TestPublishAsync_ReturnsErrorWhenCallerCtxCanceled(t *testing.T) {
	recorder := &recordingPublisher{}
	asyncPub := newAsyncPublisher(recorder, &callerCtxWorker{submitTimeoutWorker{}}, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := asyncPub.PublishAsync(ctx, PublishRequest{
		Topic: "topic",
		Value: []byte("payload"),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PublishAsync error = %v, want context.Canceled", err)
	}
	if recorder.publishCalls != 0 {
		t.Fatalf("sync publish calls = %d, want 0", recorder.publishCalls)
	}
}

// TestPublishAsync_SubmitWaitTimeoutWithRealWorker 验证真实工作池下入队等待超时后降级为同步发送
func TestPublishAsync_SubmitWaitTimeoutWithRealWorker(t *testing.T) {
	recorder := &recordingPublisher{}

	publishWorker, err := worker.NewWorker(worker.WorkConfig{
		Name:          "kafka.publish.test",
		WorkNum:       1,
		Size:          1,
		SubmitTimeout: 50 * time.Millisecond,
	}, context.Background())
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	publishWorker.Start()
	t.Cleanup(func() {
		_ = publishWorker.Close()
		publishWorker.Wait()
	})

	asyncPub := newAsyncPublisher(recorder, publishWorker, time.Second)

	// 占满唯一 worker：任务开始执行后阻塞，直到测试结束放行
	block := make(chan struct{})
	occupied := make(chan struct{})
	if err := publishWorker.TrySubmit(func(_ context.Context) error {
		close(occupied)
		<-block
		return nil
	}); err != nil {
		t.Fatalf("占用 worker 失败: %v", err)
	}
	<-occupied
	defer close(block)

	// 占满唯一队列槽位，使后续入队只能等待
	if err := publishWorker.TrySubmit(func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("占用队列槽位失败: %v", err)
	}

	start := time.Now()
	if err := asyncPub.PublishAsync(context.Background(), PublishRequest{
		Topic: "topic",
		Value: []byte("payload"),
	}); err != nil {
		t.Fatalf("PublishAsync: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("PublishAsync 未等待入队超时即返回，耗时 %v", elapsed)
	}
	if recorder.publishCalls != 1 {
		t.Fatalf("sync publish calls = %d, want 1", recorder.publishCalls)
	}
}

// TestClonePublishRequest 验证深拷贝隔离底层切片
func TestClonePublishRequest(t *testing.T) {
	original := PublishRequest{
		Topic: "topic",
		Key:   []byte("key"),
		Value: []byte("value"),
		Headers: []Header{{
			Key:   "h",
			Value: []byte("hv"),
		}},
	}
	cloned := clonePublishRequest(original)

	original.Key[0] = 'X'
	original.Value[0] = 'Y'
	original.Headers[0].Value[0] = 'Z'

	if string(cloned.Key) != "key" {
		t.Fatalf("cloned key = %q", cloned.Key)
	}
	if string(cloned.Value) != "value" {
		t.Fatalf("cloned value = %q", cloned.Value)
	}
	if string(cloned.Headers[0].Value) != "hv" {
		t.Fatalf("cloned header = %q", cloned.Headers[0].Value)
	}
}
