// Package dto 提供跨模块复用的传输结构定义与 JSON 辅助类型
//
// @author zwei
package dto

import (
	"encoding/json"
	"sync"
)

// SafeMap 表示并发安全的 string-any 映射，JSON 序列化为普通 object
type SafeMap struct {
	mu   sync.RWMutex
	data map[string]any
}

// Snapshot 返回当前映射的只读副本
func (m *SafeMap) Snapshot() map[string]any {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.data) == 0 {
		return nil
	}

	copied := make(map[string]any, len(m.data))
	for key, value := range m.data {
		copied[key] = value
	}

	return copied
}

// Set 写入键值
func (m *SafeMap) Set(key string, value any) {
	if m == nil || key == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]any)
	}

	m.data[key] = value
}

// Get 读取键值
func (m *SafeMap) Get(key string) (any, bool) {
	if m == nil || key == "" {
		return nil, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.data) == 0 {
		return nil, false
	}

	value, ok := m.data[key]
	return value, ok
}

// Delete 删除键值，映射为空时释放底层 map
func (m *SafeMap) Delete(key string) {
	if m == nil || key == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.data) == 0 {
		return
	}

	delete(m.data, key)
	if len(m.data) == 0 {
		m.data = nil
	}
}

// Replace 用给定映射整体替换当前内容
func (m *SafeMap) Replace(data map[string]any) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(data) == 0 {
		m.data = nil
		return
	}

	copied := make(map[string]any, len(data))
	for key, value := range data {
		copied[key] = value
	}

	m.data = copied
}

// MarshalJSON 将映射序列化为 JSON object，空映射序列化为 null
func (m *SafeMap) MarshalJSON() ([]byte, error) {
	snapshot := m.Snapshot()
	if len(snapshot) == 0 {
		return []byte("null"), nil
	}

	return json.Marshal(snapshot)
}

// UnmarshalJSON 从 JSON object 反序列化映射
func (m *SafeMap) UnmarshalJSON(data []byte) error {
	if m == nil {
		return nil
	}

	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}

	m.Replace(values)
	return nil
}
