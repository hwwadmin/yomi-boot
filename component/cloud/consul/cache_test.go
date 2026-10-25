// Package consul 的服务实例缓存单测

package consul

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
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

// TestServiceCacheExpiry 验证过期条目 get 返回 miss，但条目保留供 getStale 兜底
func TestServiceCacheExpiry(t *testing.T) {
	t.Parallel()

	cache := newServiceCache(30 * time.Millisecond)
	cache.set("svc", []ServiceInstance{{Address: "10.0.0.1", Port: 8080}})

	time.Sleep(80 * time.Millisecond)

	if _, ok := cache.get("svc"); ok {
		t.Fatal("过期条目应返回 miss")
	}
	stale, ok := cache.getStale("svc", time.Minute)
	if !ok || len(stale) != 1 || stale[0].Address != "10.0.0.1" {
		t.Fatalf("兜底窗口内应返回陈旧条目，实际 ok=%v stale=%v", ok, stale)
	}
	// 兜底窗口外不兜
	time.Sleep(80 * time.Millisecond)
	if _, ok := cache.getStale("svc", 50*time.Millisecond); ok {
		t.Fatal("超出兜底窗口后应返回 miss")
	}
	// 窗口关闭（0）不兜
	if _, ok := cache.getStale("svc", 0); ok {
		t.Fatal("staleWindow<=0 时应永不兜底")
	}
	// 显式删除后条目彻底消失
	cache.set("svc", []ServiceInstance{{Address: "10.0.0.1", Port: 8080}})
	cache.delete("svc")
	if _, ok := cache.getStale("svc", time.Hour); ok {
		t.Fatal("删除后不应再有陈旧条目")
	}
}

// TestNegativeCacheWindow 验证失败负缓存的窗口语义
func TestNegativeCacheWindow(t *testing.T) {
	t.Parallel()

	cache := newNegativeCache(50 * time.Millisecond)
	if _, ok := cache.get("svc"); ok {
		t.Fatal("未写入前应返回 miss")
	}

	cache.set("svc", errNoHealthyInstances)
	err, ok := cache.get("svc")
	if !ok || !errors.Is(err, errNoHealthyInstances) {
		t.Fatalf("窗口内应返回缓存的失败，实际 ok=%v err=%v", ok, err)
	}

	time.Sleep(80 * time.Millisecond)
	if _, ok := cache.get("svc"); ok {
		t.Fatal("窗口过期后应返回 miss 并清理条目")
	}
}
