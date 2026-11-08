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

func TestSafeMap_DeleteReplaceAndNil(t *testing.T) {
	var empty *SafeMap
	empty.Set("a", 1)
	empty.Delete("a")
	empty.Replace(map[string]any{"a": 1})
	if _, ok := empty.Get("a"); ok || empty.Snapshot() != nil {
		t.Fatal("nil SafeMap 应保持为空")
	}
	if err := empty.UnmarshalJSON([]byte(`{"a":1}`)); err != nil {
		t.Fatalf("nil Unmarshal: %v", err)
	}
	if raw, err := json.Marshal(empty); err != nil || string(raw) != "null" {
		t.Fatalf("nil marshal = %s err=%v", raw, err)
	}

	m := &SafeMap{}
	m.Set("", 1)
	m.Delete("")
	if _, ok := m.Get(""); ok {
		t.Fatal("空 key 不应写入")
	}
	m.Delete("missing")
	m.Set("a", 1)
	m.Set("b", 2)
	m.Delete("a")
	if _, ok := m.Get("a"); ok {
		t.Fatal("已删除的 key 仍存在")
	}
	m.Delete("b")
	if m.Snapshot() != nil {
		t.Fatal("删空后应释放映射")
	}
	m.Replace(nil)
	m.Replace(map[string]any{"c": "d"})
	if got, ok := m.Get("c"); !ok || got != "d" {
		t.Fatalf("replace = %v ok=%v", got, ok)
	}
	if err := m.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("非法 JSON 应失败")
	}
}
