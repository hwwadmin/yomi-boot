// 内存 Store 测试。
//
// @author zwei

package captcha

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestMemoryStoreContract 验证内存 Store 满足公共契约
func TestMemoryStoreContract(t *testing.T) {
	testStoreContract(t, func(t *testing.T) Store {
		store, err := NewMemoryStore(MemoryStoreConfig{})
		if err != nil {
			t.Fatalf("NewMemoryStore: %v", err)
		}
		return store
	})
}

// TestNewMemoryStoreConfig 验证内存 Store 上限配置
func TestNewMemoryStoreConfig(t *testing.T) {
	raw, err := NewMemoryStore(MemoryStoreConfig{})
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	if got := raw.(*memoryStore).maxEntries; got != defaultMaxEntries {
		t.Fatalf("maxEntries = %d, 期望 %d", got, defaultMaxEntries)
	}

	for _, maxEntries := range []int{minMaxEntries, maxMaxEntries} {
		if _, err := NewMemoryStore(MemoryStoreConfig{MaxEntries: maxEntries}); err != nil {
			t.Fatalf("MaxEntries=%d 应合法: %v", maxEntries, err)
		}
	}
	for _, maxEntries := range []int{-1, maxMaxEntries + 1} {
		if _, err := NewMemoryStore(MemoryStoreConfig{MaxEntries: maxEntries}); err == nil {
			t.Fatalf("MaxEntries=%d 应非法", maxEntries)
		}
	}
}

// TestMemoryStoreTTL 验证内存 Store 逐条检查过期时间
func TestMemoryStoreTTL(t *testing.T) {
	raw, err := NewMemoryStore(MemoryStoreConfig{})
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	store := raw.(*memoryStore)
	now := time.Unix(1_700_000_000, 0)
	store.now = func() time.Time { return now }
	ctx := context.Background()

	if err := store.Save(ctx, "before-expiry", "a", time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	now = now.Add(59 * time.Second)
	answer, found, err := store.Consume(ctx, "before-expiry")
	if err != nil || !found || answer != "a" {
		t.Fatalf("未到期 Consume = (%q, %v, %v), 期望命中", answer, found, err)
	}

	if err := store.Save(ctx, "at-expiry", "b", time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	now = now.Add(time.Minute)
	if _, found, err := store.Consume(ctx, "at-expiry"); err != nil || found {
		t.Fatalf("到期 Consume = (_, %v, %v), 期望未找到", found, err)
	}
}

// TestMemoryStoreFull 验证达到上限后的失败关闭与过期清扫
func TestMemoryStoreFull(t *testing.T) {
	raw, err := NewMemoryStore(MemoryStoreConfig{MaxEntries: 1})
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}
	store := raw.(*memoryStore)
	now := time.Unix(1_700_000_000, 0)
	store.now = func() time.Time { return now }
	ctx := context.Background()

	if err := store.Save(ctx, "one", "a", time.Minute); err != nil {
		t.Fatalf("Save one: %v", err)
	}
	if err := store.Save(ctx, "two", "b", time.Minute); !errors.Is(err, ErrStoreFull) {
		t.Fatalf("达到上限 err = %v, 期望 ErrStoreFull", err)
	}
	// 覆盖已有 id 不受上限影响
	if err := store.Save(ctx, "one", "a2", time.Minute); err != nil {
		t.Fatalf("覆盖已有 id: %v", err)
	}

	now = now.Add(time.Minute)
	if err := store.Save(ctx, "two", "b", time.Minute); err != nil {
		t.Fatalf("过期清扫后 Save: %v", err)
	}
	answer, found, err := store.Consume(ctx, "two")
	if err != nil || !found || answer != "b" {
		t.Fatalf("Consume = (%q, %v, %v), 期望命中", answer, found, err)
	}
}
