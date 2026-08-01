// Package redisstream 异步发布者单元测试
//
// @author zwei
package redisstream

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/worker"
)

// queueFullWorker 模拟工作池队列已满
type queueFullWorker struct{}

func (q *queueFullWorker) Start() {}

func (q *queueFullWorker) Submit(_ worker.WorkerHandler) error {
	return errors.New("worker.redisstream.publish.queue_full")
}

func (q *queueFullWorker) SubmitContext(_ context.Context, _ worker.WorkerHandler) error {
	return errors.New("worker.redisstream.publish.queue_full")
}

func (q *queueFullWorker) TrySubmit(_ worker.WorkerHandler) error {
	return errors.New("worker.redisstream.publish.queue_full")
}

func (q *queueFullWorker) Pending() int { return 0 }

func (q *queueFullWorker) Close() error { return nil }

func (q *queueFullWorker) Wait() {}

// recordingPublisher 记录同步 Add 是否被调用
type recordingPublisher struct {
	addCalls int
	addErr   error
}

func (p *recordingPublisher) Add(_ context.Context, _ string, _ map[string]string) (string, error) {
	p.addCalls++
	return "1-0", p.addErr
}

func (p *recordingPublisher) AddBytes(_ context.Context, _ string, _ []byte) (string, error) {
	return "1-0", nil
}

func (p *recordingPublisher) AddJSON(_ context.Context, _ string, _ any) (string, error) {
	return "1-0", nil
}

func (p *recordingPublisher) AddAsync(_ context.Context, _ string, _ map[string]string) error {
	return nil
}

func (p *recordingPublisher) AddBytesAsync(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (p *recordingPublisher) AddJSONAsync(_ context.Context, _ string, _ any) error {
	return nil
}

func (p *recordingPublisher) Close() error { return nil }

func TestShouldFallbackSync(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("worker.redisstream.publish.queue_full"), true},
		{errors.New("worker.redisstream.publish.not_run"), true},
		{errors.New("worker.redisstream.publish.stopped"), true},
		{errors.New("other"), false},
	}
	for _, tc := range cases {
		if got := shouldFallbackSync(tc.err); got != tc.want {
			t.Fatalf("shouldFallbackSync(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestAddAsync_FallbackToSyncWhenQueueFull(t *testing.T) {
	t.Parallel()
	recorder := &recordingPublisher{}
	asyncPub := NewAsyncPublisher(recorder, &queueFullWorker{}, time.Second)

	ctx := context.Background()
	if err := asyncPub.AddAsync(ctx, "orders", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("AddAsync: %v", err)
	}
	if recorder.addCalls != 1 {
		t.Fatalf("sync add calls = %d, want 1", recorder.addCalls)
	}
}

func TestAddJSONAsync_EnqueueAndCloseDrains(t *testing.T) {
	t.Parallel()
	pool, err := worker.NewWorker(worker.WorkConfig{
		Name:    "redisstream.publish.test",
		WorkNum: 1,
		Size:    8,
	}, context.Background())
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	pool.Start()

	recorder := &recordingPublisher{}
	asyncPub := NewAsyncPublisher(recorder, pool, time.Second)

	if err := asyncPub.AddJSONAsync(context.Background(), "orders", map[string]int{"n": 1}); err != nil {
		t.Fatalf("AddJSONAsync: %v", err)
	}
	if err := asyncPub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if recorder.addCalls != 1 {
		t.Fatalf("sync add calls after drain = %d, want 1", recorder.addCalls)
	}
}
