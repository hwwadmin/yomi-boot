// 令牌服务撤销、列举、会话上下文与配置校验的补充测试
//
// @author zwei
package token

import (
	"context"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	sharederr "gitee.com/hq_project/yomi-boot/component/core/shared/error"
)

// TestConfigValidate 验证令牌配置校验的各错误分支
func TestConfigValidate(t *testing.T) {
	valid := Config{TokenTTLMinutes: 30, StorePrefix: "yomi:token"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}

	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "TTL 为 0", cfg: Config{TokenTTLMinutes: 0, StorePrefix: "p"}},
		{name: "TTL 为负", cfg: Config{TokenTTLMinutes: -1, StorePrefix: "p"}},
		{name: "store_prefix 为空", cfg: Config{TokenTTLMinutes: 30}},
		{name: "skip 路径含首尾空格", cfg: Config{TokenTTLMinutes: 30, StorePrefix: "p", SkipValidatePaths: []string{" /public"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err == nil {
				t.Fatalf("%s 应返回错误", tc.name)
			}
		})
	}
}

// TestShouldSkipValidate 验证跳过校验路径的规范化匹配
func TestShouldSkipValidate(t *testing.T) {
	m := &TokenMiddleware{authConfig: Config{SkipValidatePaths: []string{"/public", " /health ", "/", "."}}}

	cases := []struct {
		path string
		want bool
	}{
		{path: "/public", want: true},
		{path: "/public/", want: true},
		{path: "/health", want: true},
		{path: "/", want: true},
		{path: "/api/secure", want: false},
		{path: "/public/other", want: false},
	}
	for _, tc := range cases {
		if got := m.shouldSkipValidate(tc.path); got != tc.want {
			t.Fatalf("shouldSkipValidate(%q) 期望 %v，实际 %v", tc.path, tc.want, got)
		}
	}
}

// TestExtractToken 验证 Bearer 令牌提取的大小写、空白与裸令牌处理
func TestExtractToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{header: "Bearer abc", want: "abc"},
		{header: "bearer abc", want: "abc"},
		{header: "  Bearer   abc  ", want: "abc"},
		{header: "raw-token", want: "raw-token"},
		{header: "", want: ""},
		{header: "   ", want: ""},
		{header: "Bearer", want: "Bearer"},
		{header: "Bearer    ", want: "Bearer"}, // 无空格分隔按裸令牌返回
	}
	for _, tc := range cases {
		if got := extractToken(tc.header); got != tc.want {
			t.Fatalf("extractToken(%q) 期望 %q，实际 %q", tc.header, tc.want, got)
		}
	}
}

// TestValidateSessionSnapshot 验证会话快照关键字段校验
func TestValidateSessionSnapshot(t *testing.T) {
	if err := validateSessionSnapshot(nil); err == nil {
		t.Fatal("nil 会话应返回错误")
	}
	if err := validateSessionSnapshot(&TokenSession{IssuedAt: time.Now()}); err == nil {
		t.Fatal("UserID 为 0 应返回错误")
	}
	if err := validateSessionSnapshot(&TokenSession{UserID: 1}); err == nil {
		t.Fatal("签发时间为零应返回错误")
	}
	if err := validateSessionSnapshot(&TokenSession{UserID: 1, IssuedAt: time.Now()}); err != nil {
		t.Fatalf("合法会话不应报错: %v", err)
	}
}

// TestServiceRevokeAndRevokeByUserID 验证按令牌与按用户撤销
func TestServiceRevokeAndRevokeByUserID(t *testing.T) {
	service, cleanup := newTestService(t, true)
	defer cleanup()
	ctx := context.Background()

	issue := func() string {
		tokenValue, err := service.Issue(ctx, &TokenSession{UserID: 5001, IssuedAt: time.Now()})
		if err != nil {
			t.Fatalf("签发失败: %v", err)
		}
		return tokenValue
	}

	if err := service.Revoke(ctx, ""); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("空令牌应返回 ErrTokenInvalid, err=%v", err)
	}

	tokenA, tokenB := issue(), issue()
	if err := service.Revoke(ctx, tokenA); err != nil {
		t.Fatalf("撤销令牌 A 失败: %v", err)
	}
	if _, err := service.Validate(ctx, tokenA); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("已撤销令牌应校验失败, err=%v", err)
	}
	if _, err := service.Validate(ctx, tokenB); err != nil {
		t.Fatalf("未撤销令牌应校验通过: %v", err)
	}

	if err := service.RevokeByUserID(ctx, 0); !errors.Is(err, sharederr.ErrInvalidInput) {
		t.Fatalf("非法 userID 应返回 ErrInvalidInput, err=%v", err)
	}
	if err := service.RevokeByUserID(ctx, 5001); err != nil {
		t.Fatalf("按用户撤销失败: %v", err)
	}
	if _, err := service.Validate(ctx, tokenB); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("按用户撤销后令牌应全部失效, err=%v", err)
	}
}

// TestServiceListByUserID 验证按用户列举令牌的排序与入参校验
func TestServiceListByUserID(t *testing.T) {
	service, cleanup := newTestService(t, true)
	defer cleanup()
	ctx := context.Background()

	if _, err := service.ListByUserID(ctx, 0); !errors.Is(err, sharederr.ErrInvalidInput) {
		t.Fatalf("非法 userID 应返回 ErrInvalidInput, err=%v", err)
	}

	// 显式递增签发时间（ExpireAt 留空由 Issue 按 TTL 补齐），验证按签发时间倒序排列
	base := time.Now().Add(-2 * time.Minute)
	for i := 0; i < 3; i++ {
		if _, err := service.Issue(ctx, &TokenSession{
			UserID:   6001,
			IssuedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("签发失败: %v", err)
		}
	}

	tokens, err := service.ListByUserID(ctx, 6001)
	if err != nil {
		t.Fatalf("列举令牌失败: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("期望 3 个令牌，实际 %d 个", len(tokens))
	}
	for i := 1; i < len(tokens); i++ {
		if tokens[i-1].Session.IssuedAt.Before(tokens[i].Session.IssuedAt) {
			t.Fatalf("列举结果应按签发时间倒序: %+v", tokens)
		}
		if tokens[i-1].TokenHash == "" || tokens[i-1].Session == nil {
			t.Fatalf("TokenHash 与会话快照应填充: %+v", tokens[i-1])
		}
	}
}

// countingTokenService 记录按用户撤销调用的令牌服务桩
type countingTokenService struct {
	mu      sync.Mutex
	revoked []int64
	err     error
}

// Issue 空实现
func (s *countingTokenService) Issue(context.Context, *TokenSession) (string, error) { return "", nil }

// Validate 空实现
func (s *countingTokenService) Validate(context.Context, string) (*TokenSession, error) {
	return nil, nil
}

// Revoke 空实现
func (s *countingTokenService) Revoke(context.Context, string) error { return nil }

// RevokeByTokenHash 空实现
func (s *countingTokenService) RevokeByTokenHash(context.Context, string) error { return nil }

// RevokeByUserID 记录撤销的 userID
func (s *countingTokenService) RevokeByUserID(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked = append(s.revoked, userID)
	return s.err
}

// TestRevokeByRelIDs 验证关联主体撤销的去重、过滤与错误透传
func TestRevokeByRelIDs(t *testing.T) {
	ctx := context.Background()

	if err := RevokeByRelIDs(ctx, nil, []int64{1}); err != nil {
		t.Fatalf("nil 服务应直接返回 nil: %v", err)
	}

	svc := &countingTokenService{}
	if err := RevokeByRelIDs(ctx, svc, []int64{1, 2, 1}, []int64{2, -1, 3}, nil); err != nil {
		t.Fatalf("批量撤销失败: %v", err)
	}
	if len(svc.revoked) != 3 || svc.revoked[0] != 1 || svc.revoked[1] != 2 || svc.revoked[2] != 3 {
		t.Fatalf("应去重并跳过非正数后按序撤销，实际 %v", svc.revoked)
	}

	failing := &countingTokenService{err: errors.New("revoke failed")}
	if err := RevokeByRelIDs(ctx, failing, []int64{1, 2}); err == nil {
		t.Fatal("撤销失败应透传错误")
	}
}

// TestRedisStoreListStaleCleanup 验证列举时自动清理索引中的残留死链与会话失效项
func TestRedisStoreListStaleCleanup(t *testing.T) {
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

// TestTokenSessionDeleteContextValue 验证自定义上下文字段删除
func TestTokenSessionDeleteContextValue(t *testing.T) {
	var nilSession *TokenSession
	nilSession.DeleteContextValue("k") // nil 接收器安全

	session := &TokenSession{}
	session.DeleteContextValue("") // 空 key 无副作用
	session.SetContextValue("k", "v")
	session.DeleteContextValue("k")
	if _, ok := session.ContextValue("k"); ok {
		t.Fatal("删除后字段应不存在")
	}
}

// errGenerator 固定返回错误的令牌生成器桩
type errGenerator struct{}

// NewToken 返回预设错误
func (errGenerator) NewToken() (string, error) { return "", errors.New("no token") }

// failingStore 固定返回错误的令牌存储桩
type failingStore struct{}

// Save 返回预设错误
func (failingStore) Save(context.Context, string, string, *TokenSession, time.Duration) error {
	return errors.New("store down")
}

// Load 空实现
func (failingStore) Load(context.Context, string, string) (*TokenSession, error) { return nil, nil }

// ListByUserID 空实现
func (failingStore) ListByUserID(context.Context, string, int64) ([]*UserTokenSession, error) {
	return nil, nil
}

// Delete 空实现
func (failingStore) Delete(context.Context, string, string) error { return nil }

// DeleteByTokenHash 空实现
func (failingStore) DeleteByTokenHash(context.Context, string, string) error { return nil }

// DeleteByUserID 空实现
func (failingStore) DeleteByUserID(context.Context, string, int64) error { return nil }

// TestIssueFailurePaths 验证生成器与存储故障时签发错误透传
func TestIssueFailurePaths(t *testing.T) {
	ctx := context.Background()

	failingGen := NewService(errGenerator{}, failingStore{}, Config{
		TokenTTLMinutes: 30, AllowMultiLogin: true, StorePrefix: "yomi:test:token",
	}, nil)
	if _, err := failingGen.Issue(ctx, &TokenSession{UserID: 1}); err == nil {
		t.Fatal("生成器故障应返回错误")
	}

	failingSave := NewService(NewUUIDGenerator(), failingStore{}, Config{
		TokenTTLMinutes: 30, AllowMultiLogin: true, StorePrefix: "yomi:test:token",
	}, nil)
	if _, err := failingSave.Issue(ctx, &TokenSession{UserID: 1}); err == nil {
		t.Fatal("存储故障应返回错误")
	}
}

// TestSessionContextOps 验证会话上下文读写与业务自定义字段操作
func TestSessionContextOps(t *testing.T) {
	ctx := context.Background()

	// 无会话时的各操作兜底
	if _, ok := SessionFromContext(ctx); ok {
		t.Fatal("无会话应返回 false")
	}
	if _, err := OperatorID(ctx); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("无会话应返回 ErrTokenInvalid, err=%v", err)
	}
	if _, ok := TokenValueFromContext(ctx); ok {
		t.Fatal("无令牌应返回 false")
	}
	if TokenContextMapFromContext(ctx) != nil {
		t.Fatal("无会话应返回 nil 上下文快照")
	}
	if _, ok := TokenContextValueFromContext(ctx, "k"); ok {
		t.Fatal("无会话读取字段应返回 false")
	}
	if SetTokenContextValue(ctx, "k", "v") {
		t.Fatal("无会话写入应返回 false")
	}
	if DeleteTokenContextValue(ctx, "k") {
		t.Fatal("无会话删除应返回 false")
	}

	// 带会话的完整往返
	session := &TokenSession{UserID: 9101, ClientType: "admin"}
	session.SetContextValue("tenant", "t1")
	ctx = WithSession(ctx, session)
	ctx = WithTokenValue(ctx, "  plain-token  ")

	got, ok := SessionFromContext(ctx)
	if !ok || got.UserID != 9101 || got.ClientType != "admin" {
		t.Fatalf("会话往返不符: %+v %v", got, ok)
	}
	if tokenValue, ok := TokenValueFromContext(ctx); !ok || tokenValue != "plain-token" {
		t.Fatalf("令牌值应裁剪后往返: %q %v", tokenValue, ok)
	}
	operatorID, err := OperatorID(ctx)
	if err != nil || operatorID != 9101 {
		t.Fatalf("OperatorID 应取会话用户: %d %v", operatorID, err)
	}
	if m := TokenContextMapFromContext(ctx); m["tenant"] != "t1" {
		t.Fatalf("上下文快照不符: %v", m)
	}
	if v, ok := TokenContextValueFromContext(ctx, "tenant"); !ok || v != "t1" {
		t.Fatalf("上下文字段读取不符: %v %v", v, ok)
	}
	if !SetTokenContextValue(ctx, "k2", "v2") {
		t.Fatal("有会话时写入应成功")
	}
	if v, ok := TokenContextValueFromContext(ctx, "k2"); !ok || v != "v2" {
		t.Fatalf("写入字段应可读回: %v %v", v, ok)
	}
	if !DeleteTokenContextValue(ctx, "k2") {
		t.Fatal("有会话时删除应成功")
	}
	if _, ok := TokenContextValueFromContext(ctx, "k2"); ok {
		t.Fatal("删除后字段应不存在")
	}

	// 空令牌值的边界
	emptyCtx := WithTokenValue(context.Background(), "   ")
	if _, ok := TokenValueFromContext(emptyCtx); ok {
		t.Fatal("空令牌值应返回 false")
	}
	zeroSession := WithSession(context.Background(), &TokenSession{})
	if _, err := OperatorID(zeroSession); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("UserID 为 0 应返回 ErrTokenInvalid, err=%v", err)
	}
}
