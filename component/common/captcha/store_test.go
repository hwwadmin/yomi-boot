// Store 契约测试。
//
// @author zwei

package captcha

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// testStoreContract 验证 Store 实现共同遵守的读写、一次性消费与并发语义
func testStoreContract(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()

	t.Run("save-and-consume", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		if err := store.Save(ctx, "id-1", "answer-1", time.Minute); err != nil {
			t.Fatalf("Save: %v", err)
		}

		answer, found, err := store.Consume(ctx, "id-1")
		if err != nil {
			t.Fatalf("Consume: %v", err)
		}
		if !found || answer != "answer-1" {
			t.Fatalf("Consume = (%q, %v), 期望 (answer-1, true)", answer, found)
		}

		if _, found, err := store.Consume(ctx, "id-1"); err != nil || found {
			t.Fatalf("二次 Consume = (_, %v, %v), 期望未找到", found, err)
		}
	})

	t.Run("unknown-id", func(t *testing.T) {
		store := newStore(t)
		answer, found, err := store.Consume(context.Background(), "missing")
		if err != nil {
			t.Fatalf("Consume: %v", err)
		}
		if found || answer != "" {
			t.Fatalf("未知 id 应未找到，实际 = (%q, %v)", answer, found)
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		if err := store.Save(ctx, "id-1", "first", time.Minute); err != nil {
			t.Fatalf("Save first: %v", err)
		}
		if err := store.Save(ctx, "id-1", "second", time.Minute); err != nil {
			t.Fatalf("Save second: %v", err)
		}

		answer, found, err := store.Consume(ctx, "id-1")
		if err != nil || !found || answer != "second" {
			t.Fatalf("Consume = (%q, %v, %v), 期望 (second, true, nil)", answer, found, err)
		}
	})

	t.Run("save-invalid", func(t *testing.T) {
		tests := []struct {
			name   string
			id     string
			answer string
			ttl    time.Duration
		}{
			{name: "空 id", id: "", answer: "a", ttl: time.Minute},
			{name: "空白 id", id: "   ", answer: "a", ttl: time.Minute},
			{name: "空 answer", id: "id", answer: "", ttl: time.Minute},
			{name: "空白 answer", id: "id", answer: "   ", ttl: time.Minute},
			{name: "ttl 为 0", id: "id", answer: "a", ttl: 0},
			{name: "ttl 为负", id: "id", answer: "a", ttl: -time.Second},
			{name: "id 过长", id: strings.Repeat("a", maxIDLen+1), answer: "a", ttl: time.Minute},
			{name: "answer 过长", id: "id", answer: strings.Repeat("a", maxAnswerLen+1), ttl: time.Minute},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := newStore(t)
				if err := store.Save(context.Background(), tt.id, tt.answer, tt.ttl); err == nil {
					t.Fatal("期望返回错误，实际为 nil")
				}
			})
		}
	})

	t.Run("consume-invalid", func(t *testing.T) {
		tests := []struct {
			name string
			id   string
		}{
			{name: "空 id", id: ""},
			{name: "空白 id", id: "   "},
			{name: "id 过长", id: strings.Repeat("a", maxIDLen+1)},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := newStore(t)
				if _, _, err := store.Consume(context.Background(), tt.id); err == nil {
					t.Fatal("期望返回错误，实际为 nil")
				}
			})
		}
	})

	t.Run("length-limits", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		id := strings.Repeat("a", maxIDLen)
		answer := strings.Repeat("b", maxAnswerLen)
		if err := store.Save(ctx, id, answer, time.Minute); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, found, err := store.Consume(ctx, id)
		if err != nil || !found || got != answer {
			t.Fatalf("Consume = (%q, %v, %v), 期望命中上限长度的记录", got, found, err)
		}
	})

	t.Run("concurrent-consume-once", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		if err := store.Save(ctx, "id-1", "answer-1", time.Minute); err != nil {
			t.Fatalf("Save: %v", err)
		}

		const workers = 32
		var hits atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				answer, found, err := store.Consume(ctx, "id-1")
				if err == nil && found && answer == "answer-1" {
					hits.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()

		if hits.Load() != 1 {
			t.Fatalf("并发 Consume 命中次数 = %d, 期望 1", hits.Load())
		}
	})

	t.Run("canceled-context", func(t *testing.T) {
		store := newStore(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if err := store.Save(ctx, "id-1", "answer-1", time.Minute); !errors.Is(err, context.Canceled) {
			t.Fatalf("Save err = %v, 期望 context.Canceled", err)
		}
		if _, _, err := store.Consume(ctx, "id-1"); !errors.Is(err, context.Canceled) {
			t.Fatalf("Consume err = %v, 期望 context.Canceled", err)
		}
	})
}
