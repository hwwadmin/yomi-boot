// Redis Store 测试。
//
// @author zwei

package captcha

import (
	"context"
	"strings"
	"testing"
	"time"

	yomiredis "github.com/hwwadmin/yomi-boot/component/common/redis"
	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"
	"github.com/hwwadmin/yomi-boot/component/core/errors"

	redisv9 "github.com/redis/go-redis/v9"
)

// newTestRedisStore 创建基于临时 Redis 的 Store
func newTestRedisStore(t *testing.T, prefix string) (Store, *redisv9.Client) {
	t.Helper()

	client, cleanup := redistest.NewClient(t)
	t.Cleanup(cleanup)

	store, err := NewRedisStore(yomiredis.NewRedisHelper(client), RedisStoreConfig{KeyPrefix: prefix})
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	return store, client
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
	store, _ := newTestRedisStore(t, "yomi:test:captcha")
	ctx := context.Background()
	const ttl = 400 * time.Millisecond

	digestA := testDigest("a")
	if err := store.Save(ctx, "beforeexpiry", digestA, ttl); err != nil {
		t.Fatalf("Save: %v", err)
	}
	time.Sleep(ttl / 2)
	answer, found, err := store.Consume(ctx, "beforeexpiry")
	if err != nil || !found || answer != digestA {
		t.Fatalf("未到期 Consume = (%q, %v, %v), 期望命中", answer, found, err)
	}

	if err := store.Save(ctx, "afterexpiry", testDigest("b"), ttl); err != nil {
		t.Fatalf("Save: %v", err)
	}
	time.Sleep(ttl + 150*time.Millisecond)
	if _, found, err := store.Consume(ctx, "afterexpiry"); err != nil || found {
		t.Fatalf("到期 Consume = (_, %v, %v), 期望未找到", found, err)
	}
}

// TestNewRedisStoreConfig 验证前缀配置与默认值
func TestNewRedisStoreConfig(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	t.Cleanup(cleanup)
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
	if err := store.Save(context.Background(), "id1", testDigest("answer-1"), time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	key := defaultKeyPrefix + ":id1"
	n, err := client.Exists(context.Background(), key).Result()
	if err != nil || n != 1 {
		keys, _ := client.Keys(context.Background(), "*").Result()
		t.Fatalf("默认前缀键不存在，现有键: %v，查询错误: %v", keys, err)
	}
}

// TestRedisStoreNeverStoresPlaintext 验证经 Service 写入 Redis 的只有答案摘要
func TestRedisStoreNeverStoresPlaintext(t *testing.T) {
	const id = "fixedid1234567890ab"
	store, client := newTestRedisStore(t, "yomi:test:captcha")
	svc := &service{driver: fixedDriver{id: id, answer: "9876"}, store: store, ttl: time.Minute}

	if _, err := svc.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	raw, err := client.Get(context.Background(), "yomi:test:captcha:"+id).Result()
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
	client, cleanup := redistest.NewClient(t)
	t.Cleanup(cleanup)
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
	if err := storeA.Save(ctx, "id1", testDigest("answer-1"), time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, found, err := storeB.Consume(ctx, "id1"); err != nil || found {
		t.Fatalf("另一个前缀不应读到记录: found=%v err=%v", found, err)
	}
	answer, found, err := storeA.Consume(ctx, "id1")
	if err != nil || !found || answer != testDigest("answer-1") {
		t.Fatalf("原前缀 Consume = (%q, %v, %v), 期望命中", answer, found, err)
	}
}

// TestRedisStoreRejectsDelimitedID 验证 id 中的冒号不能把两个前缀拼成同一个键
func TestRedisStoreRejectsDelimitedID(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	t.Cleanup(cleanup)
	helper := yomiredis.NewRedisHelper(client)

	storeA, err := NewRedisStore(helper, RedisStoreConfig{KeyPrefix: "yomi:captcha"})
	if err != nil {
		t.Fatalf("NewRedisStore A: %v", err)
	}
	storeB, err := NewRedisStore(helper, RedisStoreConfig{KeyPrefix: "yomi:captcha:login"})
	if err != nil {
		t.Fatalf("NewRedisStore B: %v", err)
	}

	ctx := context.Background()
	digest := testDigest("answer-1")
	if err := storeA.Save(ctx, "login:abc", digest, time.Minute); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("带冒号的 id 应拒绝写入: %v", err)
	}
	if _, _, err := storeA.Consume(ctx, "login:abc"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("带冒号的 id 应拒绝读取: %v", err)
	}
	got, found, err := storeB.Consume(ctx, "abc")
	if err != nil || found || got != "" {
		t.Fatalf("更长前缀不应读到被拒绝的写入: (%q, %v, %v)", got, found, err)
	}
	keys, err := client.Keys(ctx, "*").Result()
	if err != nil {
		t.Fatalf("列举 Redis 键失败: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("非法 id 不应留下 Redis 键: %v", keys)
	}
}

func TestValidateKeyPrefix_Empty(t *testing.T) {
	if err := validateKeyPrefix(""); err == nil {
		t.Fatal("空前缀应失败")
	}
}
