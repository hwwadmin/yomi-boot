package token

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTokenSession_NilReceivers(t *testing.T) {
	var session *TokenSession
	raw, err := session.MarshalJSON()
	if err != nil {
		t.Fatalf("nil MarshalJSON: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("nil MarshalJSON = %s", raw)
	}
	if err := session.UnmarshalJSON([]byte(`{}`)); err != nil {
		t.Fatalf("nil UnmarshalJSON: %v", err)
	}
	if session.ContextMap() != nil {
		t.Fatal("nil ContextMap 应返回 nil")
	}
	session.SetContextValue("k", "v")
	if _, ok := session.ContextValue("k"); ok {
		t.Fatal("nil ContextValue 应未命中")
	}
	if !session.Expired() {
		t.Fatal("nil 会话应视为已过期")
	}
	session.DeleteContextValue("k")
}

func TestTokenSession_ContextAndInvalidJSON(t *testing.T) {
	session := &TokenSession{UserID: 7, IssuedAt: time.Now(), ExpireAt: time.Now().Add(time.Hour)}
	session.SetContextValue("", "ignored")
	session.SetContextValue("tenant", "acme")
	if _, ok := session.ContextValue(""); ok {
		t.Fatal("空 key 不应命中")
	}
	if got, ok := session.ContextValue("tenant"); !ok || got != "acme" {
		t.Fatalf("context = %v ok=%v", got, ok)
	}

	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded TokenSession
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got, ok := decoded.ContextValue("tenant"); !ok || got != "acme" {
		t.Fatalf("decoded context = %v ok=%v", got, ok)
	}
	if err := decoded.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("非法 JSON 应失败")
	}
}
