// Package redisstream 发布单测
//
// @author zwei
package redisstream

import (
	"context"
	"encoding/json"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
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
