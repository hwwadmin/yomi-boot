package token

import (
	"context"
	"testing"
	"time"
)

func TestService_RejectsEmptySessionTokenAndUser(t *testing.T) {
	svc, cleanup := newTestService(t, true)
	defer cleanup()

	if _, err := svc.Issue(context.Background(), nil); err == nil {
		t.Fatal("空会话签发应失败")
	}
	if _, err := svc.Validate(context.Background(), ""); err == nil {
		t.Fatal("空令牌应失败")
	}
	if _, err := svc.ListByUserID(context.Background(), 0); err == nil {
		t.Fatal("非正用户 ID 应失败")
	}

	store, _, storeCleanup := newTestRedisStore(t)
	defer storeCleanup()
	if err := store.ReplaceUserSession(context.Background(), "yomi:test:token", "plain", nil, time.Minute); err == nil {
		t.Fatal("空会话替换应失败")
	}
}
