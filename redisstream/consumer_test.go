// Package redisstream 消费单测
//
// @author zwei
package redisstream

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/redis/redistest"

	"github.com/alicebob/miniredis/v2"
	redisv9 "github.com/redis/go-redis/v9"
)

func TestConsumer_AckFlow(t *testing.T) {
	t.Parallel()
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	zero := int64(0)
	enabled := true
	cfg := Config{
		Enabled:         &enabled,
		DefaultMaxLen:   &zero,
		BlockMS:         200,
		Count:           10,
		ClaimMinIdleMS:  1000,
		ClaimIntervalMS: 500,
	}
	components, err := NewComponents(cfg, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var got atomic.Int32
	done := make(chan struct{})
	err = components.Consumer.Start(ctx, "c1", GroupConfig{
		Stream:   "demo",
		Group:    "g1",
		Consumer: "worker-1",
		Start:    "0",
	}, func(_ context.Context, entry Entry) HandleResult {
		if string(entry.Payload()) != "hello" {
			t.Errorf("payload=%q", entry.Payload())
			return HandleDead
		}
		got.Add(1)
		close(done)
		return HandleAck
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := components.Publisher.AddBytes(ctx, "demo", []byte("hello")); err != nil {
		t.Fatalf("AddBytes: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for consume")
	}
	if got.Load() != 1 {
		t.Fatalf("got=%d want 1", got.Load())
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		pending, err := client.XPending(ctx, "demo", "g1").Result()
		if err == nil && pending.Count == 0 {
			return
		}
		if time.Now().After(deadline) {
			count := int64(-1)
			if err == nil {
				count = pending.Count
			}
			t.Fatalf("pending not cleared, err=%v count=%d", err, count)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestConsumer_RetryLeavesPELAndContinues(t *testing.T) {
	t.Parallel()
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	zero := int64(0)
	enabled := true
	cfg := Config{
		Enabled:         &enabled,
		DefaultMaxLen:   &zero,
		BlockMS:         100,
		Count:           10,
		ClaimMinIdleMS:  5000,
		ClaimIntervalMS: 5000,
	}
	components, err := NewComponents(cfg, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var retrySeen atomic.Bool
	secondDone := make(chan struct{})
	err = components.Consumer.Start(ctx, "c-retry", GroupConfig{
		Stream:   "orders",
		Group:    "g-retry",
		Consumer: "w1",
		Start:    "0",
	}, func(_ context.Context, entry Entry) HandleResult {
		switch string(entry.Payload()) {
		case "first":
			retrySeen.Store(true)
			return HandleRetry
		case "second":
			close(secondDone)
			return HandleAck
		default:
			return HandleDead
		}
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := components.Publisher.AddBytes(ctx, "orders", []byte("first")); err != nil {
		t.Fatalf("AddBytes first: %v", err)
	}
	if _, err := components.Publisher.AddBytes(ctx, "orders", []byte("second")); err != nil {
		t.Fatalf("AddBytes second: %v", err)
	}

	select {
	case <-secondDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for second message; Retry should not block consumer")
	}
	if !retrySeen.Load() {
		t.Fatal("expected first message to be handled with Retry")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		pending, err := client.XPending(ctx, "orders", "g-retry").Result()
		if err == nil && pending.Count >= 1 {
			return
		}
		if time.Now().After(deadline) {
			count := int64(-1)
			if err == nil {
				count = pending.Count
			}
			t.Fatalf("expected first message to remain in PEL, err=%v count=%d", err, count)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestConsumer_Dead(t *testing.T) {
	t.Parallel()
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	zero := int64(0)
	enabled := true
	cfg := Config{
		Enabled:         &enabled,
		DefaultMaxLen:   &zero,
		BlockMS:         100,
		Count:           10,
		ClaimMinIdleMS:  5000,
		ClaimIntervalMS: 5000,
	}
	components, err := NewComponents(cfg, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	err = components.Consumer.Start(ctx, "c-dead", GroupConfig{
		Stream:   "orders",
		Group:    "g-dead",
		Consumer: "w1",
		Start:    "0",
	}, func(_ context.Context, _ Entry) HandleResult {
		close(done)
		return HandleDead
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 业务字段与旧版扁平 meta 名冲突时仍应保留业务值
	if _, err := components.Publisher.Add(ctx, "orders", map[string]string{
		PayloadField:   "bad",
		"_original_id": "biz-keep",
		"_dead_from":   "biz-keep",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		msgs, err := client.XRange(ctx, "orders.dead", "-", "+").Result()
		if err == nil && len(msgs) >= 1 {
			msg := msgs[0]
			if got, _ := msg.Values["_original_id"].(string); got != "biz-keep" {
				t.Fatalf("_original_id overwritten: %q", got)
			}
			if got, _ := msg.Values["_dead_from"].(string); got != "biz-keep" {
				t.Fatalf("_dead_from overwritten: %q", got)
			}
			metaRaw, _ := msg.Values[DeadMetaField].(string)
			if metaRaw == "" {
				t.Fatal("expected DeadMetaField in dead message")
			}
			var meta deadMeta
			if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
				t.Fatalf("unmarshal meta: %v", err)
			}
			if meta.Stream != "orders" || meta.OriginalID == "" {
				t.Fatalf("unexpected meta: %+v", meta)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("dead stream empty, err=%v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestConsumer_StopTimeoutWhenHandlerBlocks(t *testing.T) {
	t.Parallel()
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	zero := int64(0)
	enabled := true
	components, err := NewComponents(Config{
		Enabled:         &enabled,
		DefaultMaxLen:   &zero,
		BlockMS:         100,
		ClaimIntervalMS: 60_000,
		StopTimeoutMS:   100,
	}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}

	ctx := context.Background()
	started := make(chan struct{})
	err = components.Consumer.Start(ctx, "c-block", GroupConfig{
		Stream:   "block",
		Group:    "g-block",
		Consumer: "w1",
		Start:    "0",
	}, func(_ context.Context, _ Entry) HandleResult {
		close(started)
		select {} // 故意不响应 ctx，模拟阻塞 I/O
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := components.Publisher.AddBytes(ctx, "block", []byte("x")); err != nil {
		t.Fatalf("AddBytes: %v", err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for handler start")
	}

	begin := time.Now()
	if err := components.Consumer.Stop("c-block"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(begin); elapsed > time.Second {
		t.Fatalf("Stop took too long: %v", elapsed)
	}
}

func TestConsumer_StartFailsAfterClose(t *testing.T) {
	t.Parallel()
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	zero := int64(0)
	enabled := true
	components, err := NewComponents(Config{
		Enabled:       &enabled,
		DefaultMaxLen: &zero,
		BlockMS:       100,
	}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}

	if err := components.Consumer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := components.Consumer.Close(); err != nil {
		t.Fatalf("Close idempotent: %v", err)
	}

	err = components.Consumer.Start(context.Background(), "after-close", GroupConfig{
		Stream:   "s",
		Group:    "g",
		Consumer: "w",
		Start:    "0",
	}, func(_ context.Context, _ Entry) HandleResult {
		return HandleAck
	})
	if err == nil {
		t.Fatal("expected Start to fail after Close")
	}
}

func TestConsumer_StartFailsWhenRedisUnavailable(t *testing.T) {
	t.Parallel()
	client, cleanup := redistest.NewClient(t)
	cleanup() // 关闭底层连接，使后续命令失败

	zero := int64(0)
	enabled := true
	components, err := NewComponents(Config{
		Enabled:       &enabled,
		DefaultMaxLen: &zero,
	}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	err = components.Consumer.Start(context.Background(), "c-fail", GroupConfig{
		Stream:   "s",
		Group:    "g",
		Consumer: "w",
		Start:    "0",
	}, func(_ context.Context, _ Entry) HandleResult {
		return HandleAck
	})
	if err == nil {
		t.Fatal("expected Start to fail when Redis unavailable")
	}
}

func TestConsumer_StartSucceedsWhenGroupExists(t *testing.T) {
	t.Parallel()
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	ctx := context.Background()
	if err := client.XGroupCreateMkStream(ctx, "exists", "g1", "0").Err(); err != nil {
		t.Fatalf("precreate group: %v", err)
	}

	zero := int64(0)
	enabled := true
	components, err := NewComponents(Config{
		Enabled:       &enabled,
		DefaultMaxLen: &zero,
		BlockMS:       100,
	}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	err = components.Consumer.Start(ctx, "c-exists", GroupConfig{
		Stream:   "exists",
		Group:    "g1",
		Consumer: "w1",
		Start:    "0",
	}, func(_ context.Context, _ Entry) HandleResult {
		return HandleAck
	})
	if err != nil {
		t.Fatalf("Start with existing group: %v", err)
	}
}

func TestConsumer_ClaimPending(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	stream := "claim-demo"
	group := "g-claim"
	if err := client.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}
	id, err := client.XAdd(ctx, &redisv9.XAddArgs{
		Stream: stream,
		Values: map[string]any{PayloadField: "x"},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}
	_, err = client.XReadGroup(ctx, &redisv9.XReadGroupArgs{
		Group:    group,
		Consumer: "old-worker",
		Streams:  []string{stream, ">"},
		Count:    1,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup: %v", err)
	}
	mr.FastForward(2 * time.Second)

	zero := int64(0)
	enabled := true
	cfg := Config{
		Enabled:         &enabled,
		DefaultMaxLen:   &zero,
		BlockMS:         100,
		Count:           10,
		ClaimMinIdleMS:  1,
		ClaimIntervalMS: 50,
	}
	components, err := NewComponents(cfg, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan string, 1)
	err = components.Consumer.Start(runCtx, "claimer", GroupConfig{
		Stream:       stream,
		Group:        group,
		Consumer:     "new-worker",
		Start:        "0",
		ClaimMinIdle: time.Millisecond,
	}, func(_ context.Context, entry Entry) HandleResult {
		done <- entry.ID
		return HandleAck
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case got := <-done:
		if got != id {
			t.Fatalf("claimed id=%s want %s", got, id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for claim")
	}
}
