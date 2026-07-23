// Package token 提供令牌服务的单元测试
//
// @author zwei
package token

import (
	"context"
	"sync"
	"testing"
	"time"
	"yomi-boot/distlock"
	"yomi-boot/errors"
	"yomi-boot/redis/redistest"

	yomiredis "yomi-boot/redis"

	redisv9 "github.com/redis/go-redis/v9"
)

// newTestRedisStore 创建基于 miniredis 的 RedisStore 与底层客户端
func newTestRedisStore(t *testing.T) (*RedisStore, *redisv9.Client, func()) {
	t.Helper()

	client, cleanup := redistest.NewClient(t)
	return NewRedisStore(yomiredis.NewRedisHelper(client)), client, cleanup
}

// newTestService 创建带内存 Redis 的令牌服务，allowMultiLogin 控制是否允许多端并发登录
func newTestService(t *testing.T, allowMultiLogin bool) (*Service, func()) {
	t.Helper()

	store, client, cleanup := newTestRedisStore(t)
	lockFactory, err := distlock.NewFactory(client)
	if err != nil {
		cleanup()
		t.Fatalf("NewFactory 失败: %v", err)
	}
	service := NewService(NewUUIDGenerator(), store, Config{
		TokenTTLMinutes: 30,
		AllowMultiLogin: allowMultiLogin,
		StorePrefix:     "yomi:test:token",
	}, lockFactory)

	return service, cleanup
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

// TestService_ValidateExpireAt 验证 Validate 依据会话快照中的 ExpireAt 拒绝已过期令牌，不单纯依赖 Redis TTL
func TestService_ValidateExpireAt(t *testing.T) {
	service, cleanup := newTestService(t, true)
	defer cleanup()

	ctx := context.Background()
	plainToken := "expired-token"
	session := &TokenSession{
		UserID:   3003,
		IssuedAt: time.Now().Add(-2 * time.Hour),
		ExpireAt: time.Now().Add(-time.Hour),
	}

	if err := service.store.Save(ctx, service.authConfig.StorePrefix, plainToken, session, time.Hour); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	_, err := service.Validate(ctx, plainToken)
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("期望 ErrTokenInvalid，实际 %v", err)
	}
}

// TestService_IssueRevokesPreviousWhenSingleLogin 验证单点登录模式下二次签发会撤销旧令牌，仅保留最新会话
func TestService_IssueRevokesPreviousWhenSingleLogin(t *testing.T) {
	service, cleanup := newTestService(t, false)
	defer cleanup()

	ctx := context.Background()
	userID := int64(4004)

	firstToken, err := service.Issue(ctx, &TokenSession{UserID: userID})
	if err != nil {
		t.Fatalf("首次 Issue 失败: %v", err)
	}
	if _, err := service.Validate(ctx, firstToken); err != nil {
		t.Fatalf("首次令牌应有效: %v", err)
	}

	secondToken, err := service.Issue(ctx, &TokenSession{UserID: userID})
	if err != nil {
		t.Fatalf("二次 Issue 失败: %v", err)
	}
	if secondToken == firstToken {
		t.Fatalf("二次签发应生成不同令牌")
	}

	if _, err := service.Validate(ctx, firstToken); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("单点登录下旧令牌应失效: %v", err)
	}
	if _, err := service.Validate(ctx, secondToken); err != nil {
		t.Fatalf("新令牌应有效: %v", err)
	}
}

// TestService_IssueSingleLoginConcurrent 验证单点登录模式下并发签发仅保留一个有效令牌
func TestService_IssueSingleLoginConcurrent(t *testing.T) {
	service, cleanup := newTestService(t, false)
	defer cleanup()

	ctx := context.Background()
	userID := int64(4005)
	const goroutines = 20

	var wg sync.WaitGroup
	tokens := make([]string, goroutines)
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			tokens[idx], errs[idx] = service.Issue(ctx, &TokenSession{UserID: userID})
		}(i)
	}
	wg.Wait()

	validTokenSet := make(map[string]struct{})
	for i := 0; i < goroutines; i++ {
		if errs[i] != nil {
			t.Fatalf("Issue 失败: %v", errs[i])
		}
		validTokenSet[tokens[i]] = struct{}{}
	}

	validCount := 0
	for token := range validTokenSet {
		if _, err := service.Validate(ctx, token); err == nil {
			validCount++
		}
	}
	if validCount != 1 {
		t.Fatalf("单点登录并发签发后应仅有一个有效令牌，实际 %d", validCount)
	}
}

// TestRedisStore_ListByUserIDFillsToken 验证按用户查询令牌时 UserTokenSession.Token 填充存储哈希值
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
	if tokens[0].Token != store.hashToken(plainToken) {
		t.Fatalf("Token 应填充存储哈希值，实际 %q", tokens[0].Token)
	}
	if tokens[0].Session == nil || tokens[0].Session.UserID != userID {
		t.Fatalf("Session 不符合预期: %+v", tokens[0].Session)
	}
}

// TestService_ValidateZeroExpireAt 验证 ExpireAt 零值会话在 Validate 时被拒绝
func TestService_ValidateZeroExpireAt(t *testing.T) {
	service, cleanup := newTestService(t, true)
	defer cleanup()

	ctx := context.Background()
	plainToken := "zero-expire-token"
	session := &TokenSession{
		UserID:   6006,
		IssuedAt: time.Now(),
	}

	if err := service.store.Save(ctx, service.authConfig.StorePrefix, plainToken, session, time.Hour); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	_, err := service.Validate(ctx, plainToken)
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("期望 ErrTokenInvalid，实际 %v", err)
	}
}
