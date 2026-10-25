// Package consul 的服务实例缓存单测

package consul

import (
	"testing"
	"time"
)

// TestServiceCacheGetSet 验证缓存读写与缺失返回
func TestServiceCacheGetSet(t *testing.T) {
	t.Parallel()

	cache := newServiceCache(time.Minute)
	if _, ok := cache.get("svc"); ok {
		t.Fatal("未写入前应返回 miss")
	}

	want := []ServiceInstance{{Address: "10.0.0.1", Port: 8080}, {Address: "10.0.0.2", Port: 8081}}
	cache.set("svc", want)

	got, ok := cache.get("svc")
	if !ok {
		t.Fatal("写入后应命中")
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("缓存内容期望 %v，实际 %v", want, got)
	}
}

// TestServiceCacheIsolatesCopies 验证读写均拷贝，外部修改不影响缓存
func TestServiceCacheIsolatesCopies(t *testing.T) {
	t.Parallel()

	cache := newServiceCache(time.Minute)
	source := []ServiceInstance{{Address: "10.0.0.1", Port: 8080}}
	cache.set("svc", source)

	// 写入后修改源切片，缓存不受影响
	source[0].Address = "mutated"

	// 读取后修改返回切片，缓存不受影响
	got, _ := cache.get("svc")
	got[0].Address = "mutated-too"

	again, ok := cache.get("svc")
	if !ok || again[0].Address != "10.0.0.1" {
		t.Fatalf("缓存应与外部修改隔离，实际 %v", again)
	}
}

// TestServiceCacheExpiry 验证过期条目返回 miss 并被清理
func TestServiceCacheExpiry(t *testing.T) {
	t.Parallel()

	cache := newServiceCache(30 * time.Millisecond)
	cache.set("svc", []ServiceInstance{{Address: "10.0.0.1", Port: 8080}})

	time.Sleep(80 * time.Millisecond)

	if _, ok := cache.get("svc"); ok {
		t.Fatal("过期条目应返回 miss")
	}
	cache.mu.RLock()
	remaining := len(cache.items)
	cache.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("过期条目应从缓存删除，剩余 %d 条", remaining)
	}
}
