package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewWorker_InvalidConfig(t *testing.T) {
	cases := []WorkConfig{
		{},
		{Name: "pool"},
		{Name: "pool", WorkNum: 1},
		{Name: "pool", WorkNum: 1, Size: 1, SubmitTimeout: -time.Second},
	}
	for _, cfg := range cases {
		if _, err := NewWorker(cfg, nil); err == nil {
			t.Fatalf("非法 channel 配置应失败: %+v", cfg)
		}
		if _, err := NewAntsWorker(cfg, nil); err == nil {
			t.Fatalf("非法 ants 配置应失败: %+v", cfg)
		}
	}
}

func TestWorker_GuardBranches(t *testing.T) {
	cfg := WorkConfig{Name: "guard", WorkNum: 1, Size: 1}
	channelPool, err := NewWorker(cfg, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	antsPool, err := NewAntsWorker(cfg, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = channelPool.Close()
		_ = antsPool.Close()
	})

	if err := channelPool.Submit(nil); err == nil {
		t.Fatal("空 handler 应失败")
	}
	if err := antsPool.Submit(nil); err == nil {
		t.Fatal("ants 空 handler 应失败")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := channelPool.SubmitContext(canceled, func(context.Context) error { return nil }); err == nil {
		t.Fatal("已取消的 ctx 应失败")
	}
	if err := antsPool.SubmitContext(canceled, func(context.Context) error { return nil }); err == nil {
		t.Fatal("ants 已取消的 ctx 应失败")
	}
	if err := channelPool.TrySubmit(func(context.Context) error { return nil }); err == nil {
		t.Fatal("未启动时 TrySubmit 应失败")
	}
	if err := antsPool.TrySubmit(func(context.Context) error { return nil }); err == nil {
		t.Fatal("ants 未启动时 TrySubmit 应失败")
	}

	channelPool.Start()
	channelPool.Start()
	antsPool.Start()
	antsPool.Start()

	if (&antsWorker{}).Pending() != 0 {
		t.Fatal("没有 pool 时 Pending 应为 0")
	}
	stopChannelTimer(nil)
	stopAntsTimer(nil)

	if err := channelPool.Submit(func(context.Context) error { return errors.New("job") }); err != nil {
		t.Fatal(err)
	}
	if err := antsPool.Submit(func(context.Context) error { return errors.New("job") }); err != nil {
		t.Fatal(err)
	}
	if err := channelPool.Submit(func(context.Context) error { panic("channel") }); err != nil {
		t.Fatal(err)
	}
	if err := antsPool.Submit(func(context.Context) error { panic("ants") }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := channelPool.Close(); err != nil {
		t.Fatal(err)
	}
	if err := channelPool.Close(); err != nil {
		t.Fatal(err)
	}
	if err := antsPool.Close(); err != nil {
		t.Fatal(err)
	}
	if err := antsPool.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWorker_QueueAndDirectConstructors(t *testing.T) {
	if _, err := newChannelWorker(WorkConfig{}, nil); err == nil {
		t.Fatal("channel 构造应校验配置")
	}
	if _, err := newAntsWorker(WorkConfig{}, nil); err == nil {
		t.Fatal("ants 构造应校验配置")
	}

	cfg := WorkConfig{Name: "queue", WorkNum: 1, Size: 1, SubmitTimeout: 30 * time.Millisecond}
	pool, err := NewWorker(cfg, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()

	if err := pool.SubmitContext(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("未启动时提交应失败")
	}
	if err := pool.TrySubmit(nil); err == nil {
		t.Fatal("TrySubmit 空 handler 应失败")
	}

	pool.Start()
	hold := make(chan struct{})
	started := make(chan struct{})
	if err := pool.Submit(func(context.Context) error {
		close(started)
		<-hold
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := pool.TrySubmit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := pool.TrySubmit(func(context.Context) error { return nil }); err == nil {
		t.Fatal("队列满时 TrySubmit 应失败")
	}
	if err := pool.Submit(func(context.Context) error { return nil }); err == nil {
		t.Fatal("队列满时 Submit 应超时")
	}
	close(hold)
}
