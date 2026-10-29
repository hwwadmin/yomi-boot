// Redis Store 测试。
//
// @author zwei

package captcha

import (
	"context"
	"strings"
	"testing"
	"time"

	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/alicebob/miniredis/v2"
	redisv9 "github.com/redis/go-redis/v9"
)

// newTestRedisStore 创建基于 miniredis 的 Store
func newTestRedisStore(t *testing.T, prefix string) (Store, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store, err := NewRedisStore(yomiredis.NewRedisHelper(client), RedisStoreConfig{KeyPrefix: prefix})
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	return store, mr
}

// TestRedisStoreContract 验证 Redis Store 满足公共契约
func TestRedisStoreContract(t *testing.T) {
	testStoreContract(t, func(t *testing.T) Store {
		store, _ := newTestRedisStore(t, "yomi:test:captcha")
		return store
	})
}

// TestRedisStoreTTL 验证 Redis 键按 TTL 过期
func TestRedisStoreTTL(t *testing.T) {
	store, mr := newTestRedisStore(t, "yomi:test:captcha")
	ctx := context.Background()

	digestA := testDigest("a")
	if err := store.Save(ctx, "before-expiry", digestA, time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	mr.FastForward(59 * time.Second)
	answer, found, err := store.Consume(ctx, "before-expiry")
	if err != nil || !found || answer != digestA {
		t.Fatalf("未到期 Consume = (%q, %v, %v), 期望命中", answer, found, err)
	}

	if err := store.Save(ctx, "after-expiry", testDigest("b"), time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	mr.FastForward(61 * time.Second)
	if _, found, err := store.Consume(ctx, "after-expiry"); err != nil || found {
		t.Fatalf("到期 Consume = (_, %v, %v), 期望未找到", found, err)
	}
}

// TestNewRedisStoreConfig 验证前缀配置与默认值
func TestNewRedisStoreConfig(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	helper := yomiredis.NewRedisHelper(client)

	for _, prefix := range []string{"", "yomi:test", "a_b:c1", strings.Repeat("a", maxKeyPrefixLen)} {
		if _, err := NewRedisStore(helper, RedisStoreConfig{KeyPrefix: prefix}); err != nil {
			t.Fatalf("prefix=%q 应合法: %v", prefix, err)
		}
	}
	for _, prefix := range []string{"Yomi", "yomi-captcha", ":yomi", "yomi:", "yomi captcha", strings.Repeat("a", maxKeyPrefixLen+1)} {
		_, err := NewRedisStore(helper, RedisStoreConfig{KeyPrefix: prefix})
		if err == nil {
			t.Fatalf("prefix=%q 应非法", prefix)
		}
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("prefix=%q 的错误应携带 ErrInvalidArgument: %v", prefix, err)
		}
	}
	if _, err := NewRedisStore(nil, RedisStoreConfig{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil RedisService 应返回带 ErrInvalidArgument 的错误: %v", err)
	}

	store, err := NewRedisStore(helper, RedisStoreConfig{})
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	if err := store.Save(context.Background(), "id-1", testDigest("answer-1"), time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !mr.Exists(defaultKeyPrefix + ":id-1") {
		t.Fatalf("默认前缀键不存在，现有键: %v", mr.Keys())
	}
}

// TestRedisStoreNeverStoresPlaintext 验证经 Service 写入 Redis 的只有答案摘要
func TestRedisStoreNeverStoresPlaintext(t *testing.T) {
	const id = "fixedid1234567890ab"
	store, mr := newTestRedisStore(t, "yomi:test:captcha")
	svc := &service{driver: fixedDriver{id: id, answer: "9876"}, store: store, ttl: time.Minute}

	if _, err := svc.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	raw, err := mr.Get("yomi:test:captcha:" + id)
	if err != nil {
		t.Fatalf("读取 Redis 值: %v", err)
	}
	if raw == "9876" {
		t.Fatal("Redis 中不应出现明文答案")
	}
	if want := answerDigest(normalizeAnswer("9876")); raw != want {
		t.Fatalf("Redis 值 = %q, 期望摘要 %q", raw, want)
	}
}

// TestRedisStorePrefixIsolation 验证不同前缀互不可见
func TestRedisStorePrefixIsolation(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redisv9.NewClient(&redisv9.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	helper := yomiredis.NewRedisHelper(client)

	storeA, err := NewRedisStore(helper, RedisStoreConfig{KeyPrefix: "yomi:test:a"})
	if err != nil {
		t.Fatalf("NewRedisStore A: %v", err)
	}
	storeB, err := NewRedisStore(helper, RedisStoreConfig{KeyPrefix: "yomi:test:b"})
	if err != nil {
		t.Fatalf("NewRedisStore B: %v", err)
	}

	ctx := context.Background()
	if err := storeA.Save(ctx, "id-1", testDigest("answer-1"), time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, found, err := storeB.Consume(ctx, "id-1"); err != nil || found {
		t.Fatalf("另一个前缀不应读到记录: found=%v err=%v", found, err)
	}
	answer, found, err := storeA.Consume(ctx, "id-1")
	if err != nil || !found || answer != testDigest("answer-1") {
		t.Fatalf("原前缀 Consume = (%q, %v, %v), 期望命中", answer, found, err)
	}
}
