package token

import (
	"context"
	"strconv"
	"testing"
	"time"

	yomiredis "github.com/hwwadmin/yomi-boot/component/common/redis"
	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"

	redisv9 "github.com/redis/go-redis/v9"
)

// newTestRedisStore 创建基于临时 Redis 的 RedisStore 与底层客户端
func newTestRedisStore(t *testing.T) (*RedisStore, *redisv9.Client, func()) {
	t.Helper()

	client, cleanup := redistest.NewClient(t)
	return NewRedisStore(yomiredis.NewRedisHelper(client)), client, cleanup
}

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

// TestRedisStore_TokenStoredAsHash 验证令牌以 SHA-256 哈希写入 Redis，明文不落库且不可作为查询入参
func TestRedisStore_TokenStoredAsHash(t *testing.T) {
	store, client, cleanup := newTestRedisStore(t)
	defer cleanup()

	ctx := context.Background()
	plainToken := "plain-token-value"
	session := &TokenSession{
		UserID:   1001,
		IssuedAt: time.Now(),
		ExpireAt: time.Now().Add(time.Minute),
	}

	if err := store.Save(ctx, "yomi:test:token", plainToken, session, time.Minute); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	loaded, err := store.Load(ctx, "yomi:test:token", plainToken)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if loaded == nil || loaded.UserID != session.UserID {
		t.Fatalf("Load 未返回预期会话: %+v", loaded)
	}

	plainKey := "yomi:test:token:" + plainToken
	exists, err := client.Exists(ctx, plainKey).Result()
	if err != nil {
		t.Fatalf("Exists 失败: %v", err)
	}
	if exists > 0 {
		t.Fatalf("Redis 中不应存在明文令牌 key")
	}

	hashed, err := store.Load(ctx, "yomi:test:token", store.hashToken(plainToken))
	if err != nil {
		t.Fatalf("使用哈希值 Load 失败: %v", err)
	}
	if hashed != nil {
		t.Fatalf("使用哈希值作为令牌不应命中会话")
	}
}

// TestRedisStore_IndexSetHasTTL 验证保存令牌时用户索引 SET 同步设置过期时间，避免 String key 过期后索引永久残留
func TestRedisStore_IndexSetHasTTL(t *testing.T) {
	store, client, cleanup := newTestRedisStore(t)
	defer cleanup()

	ctx := context.Background()
	ttl := 5 * time.Minute
	session := &TokenSession{
		UserID:   2002,
		IssuedAt: time.Now(),
		ExpireAt: time.Now().Add(ttl),
	}

	if err := store.Save(ctx, "yomi:test:token", "token-a", session, ttl); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	indexKey, err := store.userTokensKey("yomi:test:token", session.UserID)
	if err != nil {
		t.Fatalf("userTokensKey 失败: %v", err)
	}

	indexTTL, err := client.PTTL(ctx, indexKey).Result()
	if err != nil {
		t.Fatalf("PTTL 失败: %v", err)
	}
	if indexTTL <= 0 {
		t.Fatalf("索引 SET 应设置 TTL，实际 PTTL=%v", indexTTL)
	}
}

// TestRedisStore_ListByUserIDFillsToken 验证按用户查询令牌时 UserTokenSession.TokenHash 填充存储哈希值
func TestRedisStore_ListByUserIDFillsToken(t *testing.T) {
	store, _, cleanup := newTestRedisStore(t)
	defer cleanup()

	ctx := context.Background()
	plainToken := "list-by-user-token"
	userID := int64(5005)
	session := &TokenSession{
		UserID:   userID,
		IssuedAt: time.Now(),
		ExpireAt: time.Now().Add(time.Minute),
	}

	if err := store.Save(ctx, "yomi:test:token", plainToken, session, time.Minute); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	tokens, err := store.ListByUserID(ctx, "yomi:test:token", userID)
	if err != nil {
		t.Fatalf("ListByUserID 失败: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("期望 1 条令牌记录，实际 %d", len(tokens))
	}
	if tokens[0].TokenHash != store.hashToken(plainToken) {
		t.Fatalf("TokenHash 应填充存储哈希值，实际 %q", tokens[0].TokenHash)
	}
	if tokens[0].Session == nil || tokens[0].Session.UserID != userID {
		t.Fatalf("Session 不符合预期: %+v", tokens[0].Session)
	}
}

func TestRedisStore_DeleteLargeUserIDRemovesIndexMember(t *testing.T) {
	store, client, cleanup := newTestRedisStore(t)
	defer cleanup()

	ctx := context.Background()
	prefix := "yomi:test:token"
	otherID := int64(1001)
	otherToken := "other-user-token"
	otherSession := &TokenSession{
		UserID:   otherID,
		IssuedAt: time.Now(),
		ExpireAt: time.Now().Add(time.Hour),
	}
	if err := store.Save(ctx, prefix, otherToken, otherSession, time.Hour); err != nil {
		t.Fatalf("保存对照用户失败: %v", err)
	}

	cases := []int64{
		100000000000000,
		9007199254740993,
	}
	for _, userID := range cases {
		t.Run(strconv.FormatInt(userID, 10), func(t *testing.T) {
			tokenA := "token-a-" + strconv.FormatInt(userID, 10)
			tokenB := "token-b-" + strconv.FormatInt(userID, 10)
			session := &TokenSession{
				UserID:   userID,
				IssuedAt: time.Now(),
				ExpireAt: time.Now().Add(time.Hour),
			}
			if err := store.Save(ctx, prefix, tokenA, session, time.Hour); err != nil {
				t.Fatalf("Save A 失败: %v", err)
			}
			if err := store.Save(ctx, prefix, tokenB, session, time.Hour); err != nil {
				t.Fatalf("Save B 失败: %v", err)
			}

			if err := store.Delete(ctx, prefix, tokenA); err != nil {
				t.Fatalf("Delete 失败: %v", err)
			}

			loaded, err := store.Load(ctx, prefix, tokenA)
			if err != nil {
				t.Fatalf("Load 失败: %v", err)
			}
			if loaded != nil {
				t.Fatal("已删除令牌不应再读到会话")
			}

			indexKey, err := store.userTokensKey(prefix, userID)
			if err != nil {
				t.Fatalf("userTokensKey 失败: %v", err)
			}
			members, err := client.SMembers(ctx, indexKey).Result()
			if err != nil {
				t.Fatalf("SMEMBERS 失败: %v", err)
			}
			hashA := store.hashToken(tokenA)
			hashB := store.hashToken(tokenB)
			seenA, seenB := false, false
			for _, member := range members {
				if member == hashA {
					seenA = true
				}
				if member == hashB {
					seenB = true
				}
			}
			if seenA {
				t.Fatalf("已撤销令牌仍留在用户索引: %v", members)
			}
			if !seenB {
				t.Fatalf("未撤销令牌应留在用户索引: %v", members)
			}
		})
	}

	otherKey, err := store.userTokensKey(prefix, otherID)
	if err != nil {
		t.Fatalf("对照用户索引键失败: %v", err)
	}
	otherMembers, err := client.SMembers(ctx, otherKey).Result()
	if err != nil {
		t.Fatalf("读取对照用户索引失败: %v", err)
	}
	if len(otherMembers) != 1 || otherMembers[0] != store.hashToken(otherToken) {
		t.Fatalf("对照用户索引被改动: %v", otherMembers)
	}
}

// TestRedisStore_ReplaceUserSessionDropsPreviousTokens 验证一次替换会写上新会话，并清掉该用户其余旧令牌
func TestRedisStore_ReplaceUserSessionDropsPreviousTokens(t *testing.T) {
	store, client, cleanup := newTestRedisStore(t)
	defer cleanup()

	ctx := context.Background()
	prefix := "yomi:test:token"
	userID := int64(8008)
	otherUserID := int64(8009)
	ttl := 5 * time.Minute
	now := time.Now()

	for _, token := range []string{"old-a", "old-b"} {
		session := &TokenSession{UserID: userID, IssuedAt: now, ExpireAt: now.Add(ttl)}
		if err := store.Save(ctx, prefix, token, session, ttl); err != nil {
			t.Fatalf("Save %s 失败: %v", token, err)
		}
	}
	other := &TokenSession{UserID: otherUserID, IssuedAt: now, ExpireAt: now.Add(ttl)}
	if err := store.Save(ctx, prefix, "other-token", other, ttl); err != nil {
		t.Fatalf("Save other 失败: %v", err)
	}

	fresh := &TokenSession{UserID: userID, IssuedAt: now, ExpireAt: now.Add(ttl)}
	if err := store.ReplaceUserSession(ctx, prefix, "new-token", fresh, ttl); err != nil {
		t.Fatalf("ReplaceUserSession 失败: %v", err)
	}

	for _, token := range []string{"old-a", "old-b"} {
		loaded, err := store.Load(ctx, prefix, token)
		if err != nil {
			t.Fatalf("Load %s 失败: %v", token, err)
		}
		if loaded != nil {
			t.Fatalf("旧令牌 %s 应被删除", token)
		}
	}

	loaded, err := store.Load(ctx, prefix, "new-token")
	if err != nil {
		t.Fatalf("Load new-token 失败: %v", err)
	}
	if loaded == nil || loaded.UserID != userID {
		t.Fatalf("新令牌会话不符合预期: %+v", loaded)
	}

	otherLoaded, err := store.Load(ctx, prefix, "other-token")
	if err != nil {
		t.Fatalf("Load other-token 失败: %v", err)
	}
	if otherLoaded == nil || otherLoaded.UserID != otherUserID {
		t.Fatalf("其他用户令牌不应被替换: %+v", otherLoaded)
	}

	tokens, err := store.ListByUserID(ctx, prefix, userID)
	if err != nil {
		t.Fatalf("ListByUserID 失败: %v", err)
	}
	if len(tokens) != 1 || tokens[0].TokenHash != store.hashToken("new-token") {
		t.Fatalf("用户索引应只剩新令牌，实际 %+v", tokens)
	}

	indexKey, err := store.userTokensKey(prefix, userID)
	if err != nil {
		t.Fatalf("userTokensKey 失败: %v", err)
	}
	indexTTL, err := client.PTTL(ctx, indexKey).Result()
	if err != nil {
		t.Fatalf("PTTL 失败: %v", err)
	}
	if indexTTL <= 0 {
		t.Fatalf("替换后索引 SET 应设置 TTL，实际 PTTL=%v", indexTTL)
	}
}

// TestRedisStore_ListStaleCleanup 验证列举时自动清理索引中的残留死链与会话失效项
func TestRedisStore_ListStaleCleanup(t *testing.T) {
	store, client, cleanup := newTestRedisStore(t)
	defer cleanup()
	ctx := context.Background()
	const prefix = "yomi:test:token"

	save := func(tokenValue string, userID int64, expireAt time.Time) {
		session := &TokenSession{UserID: userID, IssuedAt: time.Now(), ExpireAt: expireAt}
		if err := store.Save(ctx, prefix, tokenValue, session, 10*time.Minute); err != nil {
			t.Fatalf("保存令牌失败: %v", err)
		}
	}
	save("live-token", 7001, time.Now().Add(10*time.Minute))
	save("gone-token", 7001, time.Now().Add(10*time.Minute))
	save("expired-token", 7001, time.Now().Add(-time.Minute))
	// 所属用户不匹配的会话（索引串写场景）
	save("wrong-user", 9999, time.Now().Add(10*time.Minute))

	// 直接删除 live 之外某一会话的存储键，模拟残留死链
	goneKey, err := store.storageKey(prefix, store.hashToken("gone-token"))
	if err != nil {
		t.Fatalf("构造存储键失败: %v", err)
	}
	if err := client.Del(ctx, goneKey).Err(); err != nil {
		t.Fatalf("删除存储键失败: %v", err)
	}

	tokens, err := store.ListByUserID(ctx, prefix, 7001)
	if err != nil {
		t.Fatalf("列举失败: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Session.UserID != 7001 {
		t.Fatalf("应仅保留有效令牌，实际 %+v", tokens)
	}

	// 索引中的残留成员应被清掉
	indexKey, err := store.userTokensKey(prefix, 7001)
	if err != nil {
		t.Fatalf("构造索引键失败: %v", err)
	}
	remaining, err := client.SMembers(ctx, indexKey).Result()
	if err != nil {
		t.Fatalf("读取索引失败: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("索引应仅剩有效成员，实际 %v", remaining)
	}
}
