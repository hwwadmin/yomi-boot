// Package token 提供令牌服务的单元测试
//
// @author zwei
package token

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/distlock"
	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"
	"github.com/hwwadmin/yomi-boot/component/core/errors"

	yomiredis "github.com/hwwadmin/yomi-boot/component/common/redis"
)

// newTestService 创建带临时 Redis 的令牌服务，allowMultiLogin 控制是否允许多端并发登录
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

// TestService_RevokeByTokenHash 验证 ListByUserID 返回的 TokenHash 配合 RevokeByTokenHash 能撤销对应明文令牌
func TestService_RevokeByTokenHash(t *testing.T) {
	service, cleanup := newTestService(t, true)
	defer cleanup()

	ctx := context.Background()
	userID := int64(7007)

	plainToken, err := service.Issue(ctx, &TokenSession{UserID: userID})
	if err != nil {
		t.Fatalf("Issue 失败: %v", err)
	}
	if _, err := service.Validate(ctx, plainToken); err != nil {
		t.Fatalf("签发的令牌应有效: %v", err)
	}

	tokens, err := service.ListByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUserID 失败: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("期望 1 条令牌记录，实际 %d", len(tokens))
	}

	if err := service.RevokeByTokenHash(ctx, tokens[0].TokenHash); err != nil {
		t.Fatalf("RevokeByTokenHash 失败: %v", err)
	}
	if _, err := service.Validate(ctx, plainToken); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("按存储哈希撤销后原令牌应失效: %v", err)
	}
}

// TestService_RevokeByTokenHashEmpty 验证空存储哈希被拒绝
func TestService_RevokeByTokenHashEmpty(t *testing.T) {
	service, cleanup := newTestService(t, true)
	defer cleanup()

	if err := service.RevokeByTokenHash(context.Background(), "  "); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("空存储哈希应返回 ErrTokenInvalid，实际 %v", err)
	}
}

// TestNewServiceFailFast 验证构造函数对 nil 依赖启动期 fail-fast
func TestNewServiceFailFast(t *testing.T) {
	store, client, cleanup := newTestRedisStore(t)
	defer cleanup()

	lockFactory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}

	cfg := Config{TokenTTLMinutes: 30, StorePrefix: "yomi:test:token"}

	expectPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("%s 应在构造期 panic", name)
			}
		}()
		fn()
	}

	expectPanic("generator 为 nil", func() {
		NewService(nil, store, cfg, lockFactory)
	})
	expectPanic("store 为 nil", func() {
		NewService(NewUUIDGenerator(), nil, cfg, lockFactory)
	})
	expectPanic("单点登录模式下 distLockFactory 为 nil", func() {
		NewService(NewUUIDGenerator(), store, cfg, nil)
	})

	// 多端登录模式不使用锁工厂，允许 nil
	cfgMulti := cfg
	cfgMulti.AllowMultiLogin = true
	if NewService(NewUUIDGenerator(), store, cfgMulti, nil) == nil {
		t.Fatal("多端登录模式下应允许 nil 锁工厂")
	}
}

// scriptFailRedis 在替换脚本上返回错误，其余命令交给真实 Redis
type scriptFailRedis struct {
	yomiredis.RedisService
	failReplace bool
}

// OpsScript4Eval 拦截单点登录替换脚本，模拟提交失败且不改动已有会话
func (r *scriptFailRedis) OpsScript4Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	if r.failReplace && strings.Contains(script, "luaReplaceUserTokenSession") {
		return nil, errors.New("脚本执行失败")
	}
	return r.RedisService.OpsScript4Eval(ctx, script, keys, args...)
}

// TestService_IssueReplaceFailureKeepsPreviousToken 验证单点登录替换提交失败时，旧令牌仍然有效
func TestService_IssueReplaceFailureKeepsPreviousToken(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	gate := &scriptFailRedis{RedisService: yomiredis.NewRedisHelper(client)}
	store := NewRedisStore(gate)
	lockFactory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("NewFactory 失败: %v", err)
	}
	service := NewService(NewUUIDGenerator(), store, Config{
		TokenTTLMinutes: 30,
		AllowMultiLogin: false,
		StorePrefix:     "yomi:test:token",
	}, lockFactory)

	ctx := context.Background()
	userID := int64(9009)
	firstToken, err := service.Issue(ctx, &TokenSession{UserID: userID})
	if err != nil {
		t.Fatalf("首次 Issue 失败: %v", err)
	}

	gate.failReplace = true
	if _, err := service.Issue(ctx, &TokenSession{UserID: userID}); err == nil {
		t.Fatal("替换提交失败应返回错误")
	}
	if _, err := service.Validate(ctx, firstToken); err != nil {
		t.Fatalf("替换失败时旧令牌应仍然有效: %v", err)
	}

	tokens, err := service.ListByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUserID 失败: %v", err)
	}
	if len(tokens) != 1 || tokens[0].TokenHash != store.hashToken(firstToken) {
		t.Fatalf("替换失败后应仍只有旧令牌，实际 %+v", tokens)
	}
}

// TestNewRedisStoreNilFailFast 验证 nil Redis 服务在构造期 panic
func TestNewRedisStoreNilFailFast(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("redisService 为 nil 应在构造期 panic")
		}
	}()
	NewRedisStore(nil)
}
