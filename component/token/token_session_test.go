// Package token 提供令牌会话模型的单元测试
//
// @author zwei
package token

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// TestTokenSession_ContextJSONRoundTrip 验证 TokenSession 自定义 Context 经 JSON 往返后键值保持一致
func TestTokenSession_ContextJSONRoundTrip(t *testing.T) {
	session := &TokenSession{
		UserID: 1001,
	}
	session.SetContextValue("tenant", "demo")
	session.SetContextValue("role", float64(1))

	payload, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	loaded := &TokenSession{}
	if err := json.Unmarshal(payload, loaded); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}

	value, ok := loaded.ContextValue("tenant")
	if !ok || value != "demo" {
		t.Fatalf("tenant 字段不符合预期: ok=%v value=%v", ok, value)
	}
	value, ok = loaded.ContextValue("role")
	if !ok || value != float64(1) {
		t.Fatalf("role 字段不符合预期: ok=%v value=%v", ok, value)
	}
}

// TestTokenSession_ContextMapReturnsSnapshot 验证 ContextMap 返回副本，修改返回值不影响会话内部上下文
func TestTokenSession_ContextMapReturnsSnapshot(t *testing.T) {
	session := &TokenSession{}
	session.SetContextValue("key", "value")

	snapshot := session.ContextMap()
	snapshot["key"] = "changed"

	value, ok := session.ContextValue("key")
	if !ok || value != "value" {
		t.Fatalf("ContextMap 应返回副本，不应影响原始上下文: ok=%v value=%v", ok, value)
	}
}

// TestTokenSession_ContextConcurrentAccess 验证多 goroutine 并发读写 Context 不会触发 panic
func TestTokenSession_ContextConcurrentAccess(t *testing.T) {
	session := &TokenSession{}
	const goroutines = 32

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			session.SetContextValue("write", idx)
		}(i)
		go func(idx int) {
			defer wg.Done()
			_, _ = session.ContextValue("write")
		}(i)
		go func(idx int) {
			defer wg.Done()
			_ = session.ContextMap()
		}(i)
	}
	wg.Wait()
}

// TestTokenSession_ExpiredZeroExpireAt 验证 ExpireAt 零值会话视为已过期，避免永不过期
func TestTokenSession_ExpiredZeroExpireAt(t *testing.T) {
	session := &TokenSession{
		UserID:   1001,
		IssuedAt: time.Now(),
	}
	if !session.Expired() {
		t.Fatal("ExpireAt 为零时应视为已过期")
	}

	future := &TokenSession{
		UserID:   1001,
		IssuedAt: time.Now(),
		ExpireAt: time.Now().Add(time.Hour),
	}
	if future.Expired() {
		t.Fatal("未过期的会话不应被判定为已过期")
	}
}
