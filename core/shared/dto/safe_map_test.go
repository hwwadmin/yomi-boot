// Package dto 提供跨模块复用的传输结构定义与 JSON 辅助类型
//
// @author zwei
package dto

import (
	"encoding/json"
	"sync"
	"testing"
)

// TestSafeMap_JSONRoundTrip 验证 SafeMap 经 JSON 序列化与反序列化后键值保持一致
func TestSafeMap_JSONRoundTrip(t *testing.T) {
	m := &SafeMap{}
	m.Set("tenant", "demo")
	m.Set("role", float64(1))

	payload, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	loaded := &SafeMap{}
	if err := json.Unmarshal(payload, loaded); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}

	value, ok := loaded.Get("tenant")
	if !ok || value != "demo" {
		t.Fatalf("tenant 字段不符合预期: ok=%v value=%v", ok, value)
	}
	value, ok = loaded.Get("role")
	if !ok || value != float64(1) {
		t.Fatalf("role 字段不符合预期: ok=%v value=%v", ok, value)
	}
}

// TestSafeMap_SnapshotIsolated 验证 Snapshot 返回副本，修改快照不影响内部映射
func TestSafeMap_SnapshotIsolated(t *testing.T) {
	m := &SafeMap{}
	m.Set("key", "value")

	snapshot := m.Snapshot()
	snapshot["key"] = "changed"

	value, ok := m.Get("key")
	if !ok || value != "value" {
		t.Fatalf("Snapshot 应返回副本: ok=%v value=%v", ok, value)
	}
}

// TestSafeMap_ConcurrentAccess 验证多 goroutine 并发读写 Snapshot 不会触发 panic
func TestSafeMap_ConcurrentAccess(t *testing.T) {
	m := &SafeMap{}
	const goroutines = 32

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			m.Set("write", idx)
		}(i)
		go func(idx int) {
			defer wg.Done()
			_, _ = m.Get("write")
		}(i)
		go func(idx int) {
			defer wg.Done()
			_ = m.Snapshot()
		}(i)
	}
	wg.Wait()
}
