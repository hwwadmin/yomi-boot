package token

import (
	"context"
	"testing"
	"time"
)

func TestRedisStore_RejectsBadRecords(t *testing.T) {
	store, _, cleanup := newTestRedisStore(t)
	defer cleanup()
	ctx := context.Background()
	session := &TokenSession{UserID: 7, IssuedAt: time.Now()}

	if err := store.Save(ctx, "pfx", "tok", nil, time.Minute); err == nil {
		t.Fatal("空会话不应写入")
	}
	if err := store.Save(ctx, ":", "tok", session, time.Minute); err == nil {
		t.Fatal("空前缀不应写入")
	}
	if err := store.Save(ctx, "pfx", "tok", &TokenSession{IssuedAt: time.Now()}, time.Minute); err == nil {
		t.Fatal("非正用户 ID 不应写入")
	}
	bad := &TokenSession{UserID: 7, IssuedAt: time.Now()}
	bad.SetContextValue("ch", make(chan int))
	if err := store.Save(ctx, "pfx", "tok", bad, time.Minute); err == nil {
		t.Fatal("不可序列化的会话应失败")
	}

	if err := store.ReplaceUserSession(ctx, "pfx", "tok", nil, time.Minute); err == nil {
		t.Fatal("空会话不应替换")
	}
	if err := store.ReplaceUserSession(ctx, ":", "tok", session, time.Minute); err == nil {
		t.Fatal("空前缀不应替换")
	}
	if _, err := store.Load(ctx, ":", "tok"); err == nil {
		t.Fatal("空前缀不应读取")
	}
	if _, err := store.ListByUserID(ctx, ":", 7); err == nil {
		t.Fatal("空前缀不应列举")
	}
	if err := store.DeleteByTokenHash(ctx, "pfx", " "); err == nil {
		t.Fatal("空存储哈希不应删除")
	}
	if err := store.DeleteByUserID(ctx, "pfx", 0); err == nil {
		t.Fatal("非正用户 ID 不应删除")
	}
	if _, err := store.storageKey("pfx", " "); err == nil {
		t.Fatal("空白存储后缀应失败")
	}
}
