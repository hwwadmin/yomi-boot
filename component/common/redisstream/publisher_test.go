// Package redisstream 发布单测
//
// @author zwei
package redisstream

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"

	redisv9 "github.com/redis/go-redis/v9"
)

func TestPublisher_AddBytesAndJSON(t *testing.T) {
	t.Parallel()
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	zero := int64(0)
	cfg := Config{DefaultMaxLen: &zero, KeyPrefix: "rs:"}
	pub := NewPublisher(client, toRuntimeConfig(cfg))

	ctx := context.Background()
	id, err := pub.AddBytes(ctx, "orders", []byte(`{"n":1}`))
	if err != nil {
		t.Fatalf("AddBytes: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	type payload struct {
		N int `json:"n"`
	}
	id2, err := pub.AddJSON(ctx, "orders", payload{N: 2})
	if err != nil {
		t.Fatalf("AddJSON: %v", err)
	}
	if id2 == "" || id2 == id {
		t.Fatalf("expected distinct id, got %q after %q", id2, id)
	}

	msgs, err := client.XRange(ctx, "rs:orders", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	raw, _ := msgs[1].Values[PayloadField].(string)
	var got payload
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.N != 2 {
		t.Fatalf("payload.N=%d want 2", got.N)
	}
}

func TestPublisher_RejectsEmpty(t *testing.T) {
	t.Parallel()
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	pub := NewPublisher(client, toRuntimeConfig(Config{}))
	if _, err := pub.Add(context.Background(), "s", nil); err == nil {
		t.Fatal("expected error for empty values")
	}
	if _, err := pub.AddBytes(context.Background(), "", []byte("x")); err == nil {
		t.Fatal("expected error for empty stream")
	}
}

func TestNewPublisher_NilClientPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil client 应 panic")
		}
	}()
	NewPublisher(nil, RuntimeConfig{})
}

func TestPublisher_AddFailsWhenRedisDown(t *testing.T) {
	client := redisv9.NewClient(&redisv9.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	p := NewPublisher(client, RuntimeConfig{DefaultMaxLen: 10})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := p.Add(ctx, "s", map[string]string{"k": "v"}); err == nil {
		t.Fatal("连不上 Redis 时发布应失败")
	}
}
