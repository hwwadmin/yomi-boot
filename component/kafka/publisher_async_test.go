// Package kafka 提供 Kafka 异步发布者单元测试
//
// @author zwei
package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/worker"
)

// queueFullWorker 模拟工作池队列已满
type queueFullWorker struct{}

// Start 实现 Start 接口（测试桩）
func (q *queueFullWorker) Start() {}

// Submit 实现 Submit 接口（测试桩，模拟队列已满）
func (q *queueFullWorker) Submit(_ worker.WorkerHandler) error {
	return errors.New("worker.kafka.publish.queue_full")
}

// SubmitContext 实现 SubmitContext 接口（测试桩，模拟队列已满）
func (q *queueFullWorker) SubmitContext(_ context.Context, _ worker.WorkerHandler) error {
	return errors.New("worker.kafka.publish.queue_full")
}

// TrySubmit 实现 TrySubmit 接口（测试桩，模拟队列已满）
func (q *queueFullWorker) TrySubmit(_ worker.WorkerHandler) error {
	return errors.New("worker.kafka.publish.queue_full")
}

// Pending 实现 Pending 接口（测试桩）
func (q *queueFullWorker) Pending() int { return 0 }

// Close 实现 Close 接口（测试桩）
func (q *queueFullWorker) Close() error { return nil }

// Wait 实现 Wait 接口（测试桩）
func (q *queueFullWorker) Wait() {}

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
		{errors.New("other"), false},
	}
	for _, tc := range cases {
		if got := shouldFallbackSync(tc.err); got != tc.want {
			t.Fatalf("shouldFallbackSync(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// TestPublishAsync_FallbackToSyncWhenQueueFull 验证队列满时降级调用同步 Publish
func TestPublishAsync_FallbackToSyncWhenQueueFull(t *testing.T) {
	recorder := &recordingPublisher{}
	asyncPub := NewAsyncPublisher(recorder, &queueFullWorker{}, time.Second)

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
