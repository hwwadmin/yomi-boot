package redis

import (
	"context"
	"net"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"

	redisv9 "github.com/redis/go-redis/v9"
)

// TestRedisHelper_OpsStr4SetAndGet 验证 String 写入与读取
func TestRedisHelper_OpsStr4SetAndGet(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:str"

	if err := helper.OpsStr4Set(ctx, key, "hello", time.Minute); err != nil {
		t.Fatalf("OpsStr4Set 失败: %v", err)
	}

	value, exists, err := helper.OpsStr4Get(ctx, key)
	if err != nil {
		t.Fatalf("OpsStr4Get 失败: %v", err)
	}
	if !exists || value != "hello" {
		t.Fatalf("OpsStr4Get 期望 hello,true，实际 %s,%v", value, exists)
	}
}

// TestRedisHelper_OpsKey4ExistsAndDelete 验证 Key 存在判断与删除
func TestRedisHelper_OpsKey4ExistsAndDelete(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:exists"

	if err := helper.OpsStr4SetNoTTL(ctx, key, "1"); err != nil {
		t.Fatalf("OpsStr4SetNoTTL 失败: %v", err)
	}

	exists, err := helper.OpsKey4Exists(ctx, key)
	if err != nil || !exists {
		t.Fatalf("OpsKey4Exists 期望 true,nil，实际 %v,%v", exists, err)
	}

	if err := helper.OpsKey4Delete(ctx, key); err != nil {
		t.Fatalf("OpsKey4Delete 失败: %v", err)
	}

	exists, err = helper.OpsKey4Exists(ctx, key)
	if err != nil || exists {
		t.Fatalf("删除后 OpsKey4Exists 期望 false,nil，实际 %v,%v", exists, err)
	}
}

// TestRedisHelper_OpsSet4AddAndMembers 验证 Set 写入与读取
func TestRedisHelper_OpsSet4AddAndMembers(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:set"

	if err := helper.OpsSet4Add(ctx, key, "a", "b"); err != nil {
		t.Fatalf("OpsSet4Add 失败: %v", err)
	}

	members, err := helper.OpsSet4Members(ctx, key)
	if err != nil {
		t.Fatalf("OpsSet4Members 失败: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("OpsSet4Members 期望 2 个成员，实际 %d", len(members))
	}
}

// TestRedisHelper_RejectsNegativeTTL 验证负 TTL 被拒绝且不写入数据
func TestRedisHelper_RejectsNegativeTTL(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()

	if err := helper.OpsStr4Set(ctx, "yomi:test:negttl:str", "v", -time.Second); err == nil {
		t.Fatal("OpsStr4Set 期望负 TTL 返回错误")
	}
	if err := helper.OpsStr4SetJSON(ctx, "yomi:test:negttl:json", map[string]string{"a": "1"}, -time.Second); err == nil {
		t.Fatal("OpsStr4SetJSON 期望负 TTL 返回错误")
	}
	if err := helper.OpsHash4Set(ctx, "yomi:test:negttl:hash", map[string]any{"f": "v"}, -time.Second); err == nil {
		t.Fatal("OpsHash4Set 期望负 TTL 返回错误")
	}

	exists, err := helper.OpsKey4Exists(ctx, "yomi:test:negttl:str")
	if err != nil || exists {
		t.Fatalf("负 TTL 的 String 键不应写入，实际 exists=%v err=%v", exists, err)
	}
	exists, err = helper.OpsKey4Exists(ctx, "yomi:test:negttl:hash")
	if err != nil || exists {
		t.Fatalf("负 TTL 的 Hash 键不应写入，实际 exists=%v err=%v", exists, err)
	}
}

// TestRedisHelper_RejectsNonPositiveExpireTTL 验证非正 TTL 的 Expire 被拒绝且不删键
func TestRedisHelper_RejectsNonPositiveExpireTTL(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:expire:guard"

	if err := helper.OpsStr4Set(ctx, key, "v", time.Hour); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}

	applied, err := helper.OpsKey4Expire(ctx, key, 0)
	if err == nil || applied {
		t.Fatalf("OpsKey4Expire 期望 ttl=0 返回错误,applied=false，实际 %v,%v", applied, err)
	}
	applied, err = helper.OpsKey4Expire(ctx, key, -time.Second)
	if err == nil || applied {
		t.Fatalf("OpsKey4Expire 期望负 TTL 返回错误,applied=false，实际 %v,%v", applied, err)
	}

	exists, err := helper.OpsKey4Exists(ctx, key)
	if err != nil || !exists {
		t.Fatalf("非正 TTL 不应删键，实际 exists=%v err=%v", exists, err)
	}
}

// TestNewRedisHelper_NilClientPanics 验证未初始化客户端时构造助手会 panic
func TestNewRedisHelper_NilClientPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("期望 panic")
		}
	}()
	NewRedisHelper(nil)
}

// TestRedisHelper_CommandTimeout 验证单条指令超过最长等待时间后返回错误
func TestRedisHelper_CommandTimeout(t *testing.T) {
	addr, stop := startHangingRedisServer(t)
	defer stop()

	client := redisv9.NewClient(&redisv9.Options{
		Addr:                  addr,
		ReadTimeout:           5 * time.Second,
		WriteTimeout:          5 * time.Second,
		ContextTimeoutEnabled: true,
	})
	defer func() {
		_ = client.Close()
	}()

	helper := NewRedisHelper(client, WithCommandTimeout(100*time.Millisecond))

	start := time.Now()
	_, _, err := helper.OpsStr4Get(context.Background(), "yomi:test:timeout")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("期望指令超时返回错误")
	}
	if elapsed > time.Second {
		t.Fatalf("指令超时未生效，耗时 %v", elapsed)
	}
}

// TestRedisHelper_OpsRateLimit4Allow_MaxN 验证单次消耗 n 超上限报错、恰好等于上限正常放行
func TestRedisHelper_OpsRateLimit4Allow_MaxN(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:rl:maxn"

	if _, err := helper.OpsRateLimit4Allow(ctx, key, 100, maxRateLimitN+1, time.Minute); err == nil {
		t.Fatal("n 超过单次上限应返回错误")
	}

	allowed, err := helper.OpsRateLimit4Allow(ctx, key, maxRateLimitN*2, maxRateLimitN, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("n 恰好等于上限应放行，实际 %v,%v", allowed, err)
	}
}

// startHangingRedisServer 启动只接受连接、不应答任何指令的 TCP 服务，用于验证指令超时
func startHangingRedisServer(t *testing.T) (addr string, stop func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听本地端口失败: %v", err)
	}

	release := make(chan struct{})
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer func() {
					_ = conn.Close()
				}()
				<-release
			}()
		}
	}()

	return listener.Addr().String(), func() {
		close(release)
		_ = listener.Close()
	}
}

// TestRedisHelper_OpsStr4JSONRoundTrip 验证 JSON String 的写入读取往返与异常分支
func TestRedisHelper_OpsStr4JSONRoundTrip(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:json"

	payload := map[string]any{"name": "yomi", "count": 3}
	if err := helper.OpsStr4SetJSON(ctx, key, payload, time.Minute); err != nil {
		t.Fatalf("OpsStr4SetJSON 失败: %v", err)
	}
	if err := helper.OpsStr4SetJSONNoTTL(ctx, key+":nottl", payload); err != nil {
		t.Fatalf("OpsStr4SetJSONNoTTL 失败: %v", err)
	}

	var got map[string]any
	exists, err := helper.OpsStr4GetJSON(ctx, key, &got)
	if err != nil || !exists {
		t.Fatalf("OpsStr4GetJSON 期望 exists=true，实际 %v,%v", exists, err)
	}
	if got["name"] != "yomi" {
		t.Fatalf("JSON 往返数据不符: %+v", got)
	}

	// 无 TTL 写入
	exists, err = helper.OpsStr4GetJSON(ctx, key+":nottl", &got)
	if err != nil || !exists {
		t.Fatalf("OpsStr4GetJSON(NoTTL) 期望 exists=true，实际 %v,%v", exists, err)
	}

	// 键不存在返回 exists=false 且不报错
	exists, err = helper.OpsStr4GetJSON(ctx, "yomi:test:json:missing", &got)
	if err != nil || exists {
		t.Fatalf("缺失键期望 false,nil，实际 %v,%v", exists, err)
	}

	// 反序列化目标为空返回错误
	if _, err := helper.OpsStr4GetJSON(ctx, key, nil); err == nil {
		t.Fatal("反序列化目标为空应返回错误")
	}

	// 非法 JSON 返回错误
	if err := helper.OpsStr4SetNoTTL(ctx, key+":bad", "not-json"); err != nil {
		t.Fatalf("准备非法 JSON 失败: %v", err)
	}
	if _, err := helper.OpsStr4GetJSON(ctx, key+":bad", &got); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
}

// TestRedisHelper_OpsHash4SetAndGet 验证 Hash 的原子写入、读取与空字段跳过
func TestRedisHelper_OpsHash4SetAndGet(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:hash"

	if err := helper.OpsHash4Set(ctx, key, map[string]any{"f1": "v1", "f2": "v2"}, time.Minute); err != nil {
		t.Fatalf("OpsHash4Set 失败: %v", err)
	}

	value, exists, err := helper.OpsHash4Get(ctx, key, "f1")
	if err != nil || !exists || value != "v1" {
		t.Fatalf("OpsHash4Get 期望 v1,true，实际 %s,%v,%v", value, exists, err)
	}
	_, exists, err = helper.OpsHash4Get(ctx, key, "missing")
	if err != nil || exists {
		t.Fatalf("缺失字段期望 false,nil，实际 %v,%v", exists, err)
	}

	values, exists, err := helper.OpsHash4GetAll(ctx, key)
	if err != nil || !exists {
		t.Fatalf("OpsHash4GetAll 期望 exists=true，实际 %v,%v", exists, err)
	}
	if values["f1"] != "v1" || values["f2"] != "v2" {
		t.Fatalf("OpsHash4GetAll 数据不符: %+v", values)
	}

	// 键不存在返回 exists=false 且不报错
	values, exists, err = helper.OpsHash4GetAll(ctx, "yomi:test:hash:missing")
	if err != nil || exists || values != nil {
		t.Fatalf("缺失键期望 nil,false,nil，实际 %v,%v,%v", values, exists, err)
	}

	// 空 fields 不建键
	if err := helper.OpsHash4Set(ctx, "yomi:test:hash:empty", map[string]any{}, time.Minute); err != nil {
		t.Fatalf("空 fields 不应报错: %v", err)
	}
	exists, err = helper.OpsKey4Exists(ctx, "yomi:test:hash:empty")
	if err != nil || exists {
		t.Fatalf("空 fields 不应建键，实际 exists=%v err=%v", exists, err)
	}
}

// TestRedisHelper_OpsKey4Expire 验证为存在的键续期与缺失键返回 applied=false
func TestRedisHelper_OpsKey4Expire(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:expire"

	if err := helper.OpsStr4SetNoTTL(ctx, key, "v"); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}
	applied, err := helper.OpsKey4Expire(ctx, key, time.Minute)
	if err != nil || !applied {
		t.Fatalf("存在键应 applied=true，实际 %v,%v", applied, err)
	}
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		t.Fatalf("续期后应有过期时间，实际 %v,%v", ttl, err)
	}

	applied, err = helper.OpsKey4Expire(ctx, "yomi:test:expire:missing", time.Minute)
	if err != nil || applied {
		t.Fatalf("缺失键期望 applied=false,nil，实际 %v,%v", applied, err)
	}
}

// TestRedisHelper_OpsSet4Remove 验证 Set 成员移除与空参数跳过
func TestRedisHelper_OpsSet4Remove(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:set:remove"

	if err := helper.OpsSet4Add(ctx, key, "a", "b", "c"); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}
	if err := helper.OpsSet4Remove(ctx, key, "b"); err != nil {
		t.Fatalf("OpsSet4Remove 失败: %v", err)
	}
	members, err := helper.OpsSet4Members(ctx, key)
	if err != nil || len(members) != 2 {
		t.Fatalf("移除后期望 2 个成员，实际 %d,%v", len(members), err)
	}

	if err := helper.OpsSet4Remove(ctx, key); err != nil {
		t.Fatalf("空参数不应报错: %v", err)
	}
	if err := helper.OpsSet4Add(ctx, "yomi:test:set:empty:add"); err != nil {
		t.Fatalf("空参数不应报错: %v", err)
	}
}

// TestRedisHelper_OpsKey4DeleteEmpty 验证空键列表直接返回
func TestRedisHelper_OpsKey4DeleteEmpty(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	if err := helper.OpsKey4Delete(context.Background()); err != nil {
		t.Fatalf("空键列表不应报错: %v", err)
	}
}

// TestRedisHelper_WithCommandTimeoutKeepsDefault 验证非正超时不覆盖默认值
func TestRedisHelper_WithCommandTimeoutKeepsDefault(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client, WithCommandTimeout(0), nil)
	if helper.commandTimeout != defaultCommandTimeout {
		t.Fatalf("非正超时应保留默认值，实际 %v", helper.commandTimeout)
	}
}

// TestRedisHelper_OpsRateLimit4Allow_Guards 验证限流入参守卫分支
func TestRedisHelper_OpsRateLimit4Allow_Guards(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()

	cases := []struct {
		name   string
		key    string
		limit  int64
		n      int64
		window time.Duration
	}{
		{name: "key 为空", key: "  ", limit: 10, n: 1, window: time.Minute},
		{name: "limit 非正", key: "k", limit: 0, n: 1, window: time.Minute},
		{name: "n 非正", key: "k", limit: 10, n: 0, window: time.Minute},
		{name: "window 非正", key: "k", limit: 10, n: 1, window: 0},
		{name: "window 毫秒后为 0", key: "k", limit: 10, n: 1, window: 500 * time.Microsecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := helper.OpsRateLimit4Allow(ctx, tc.key, tc.limit, tc.n, tc.window); err == nil {
				t.Fatalf("%s 应返回错误", tc.name)
			}
		})
	}

	// n > limit 直接拒绝且不报错
	allowed, err := helper.OpsRateLimit4Allow(ctx, "yomi:test:rl:guard", 2, 3, time.Minute)
	if err != nil || allowed {
		t.Fatalf("n>limit 期望 false,nil，实际 %v,%v", allowed, err)
	}
}

// TestRedisHelper_OpsRateLimit4Allow_Exhausted 验证窗口内配额耗尽后拒绝放行
func TestRedisHelper_OpsRateLimit4Allow_Exhausted(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	ctx := context.Background()
	key := "yomi:test:rl:exhausted"

	for i := 0; i < 3; i++ {
		allowed, err := helper.OpsRateLimit4Allow(ctx, key, 3, 1, time.Minute)
		if err != nil || !allowed {
			t.Fatalf("第 %d 次消耗应放行，实际 %v,%v", i+1, allowed, err)
		}
	}
	allowed, err := helper.OpsRateLimit4Allow(ctx, key, 3, 1, time.Minute)
	if err != nil || allowed {
		t.Fatalf("配额耗尽应返回 false,nil，实际 %v,%v", allowed, err)
	}
}

// TestRedisHelper_OpsRateLimit4Allow_NilHelper 验证未初始化助手返回错误
func TestRedisHelper_OpsRateLimit4Allow_NilHelper(t *testing.T) {
	var helper *RedisHelper
	if _, err := helper.OpsRateLimit4Allow(context.Background(), "k", 10, 1, time.Minute); err == nil {
		t.Fatal("未初始化助手应返回错误")
	}
}

// TestRedisHelper_OpsStr4SetJSON_MarshalFailure 验证不可序列化对象返回错误
func TestRedisHelper_OpsStr4SetJSON_MarshalFailure(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := NewRedisHelper(client)
	if err := helper.OpsStr4SetJSON(context.Background(), "yomi:test:json:chan", map[string]any{"ch": make(chan int)}, time.Minute); err == nil {
		t.Fatal("不可序列化对象应返回错误")
	}
}

// TestRedisHelper_ErrorPathsOnClosedServer 验证服务不可用时各操作返回错误而非 panic
func TestRedisHelper_ErrorPathsOnClosedServer(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	helper := NewRedisHelper(client)
	cleanup()

	ctx := context.Background()
	if err := helper.OpsStr4Set(ctx, "k", "v", time.Minute); err == nil {
		t.Fatal("OpsStr4Set 期望错误")
	}
	if _, _, err := helper.OpsStr4Get(ctx, "k"); err == nil {
		t.Fatal("OpsStr4Get 期望错误")
	}
	if err := helper.OpsStr4SetJSON(ctx, "k", map[string]string{"a": "1"}, time.Minute); err == nil {
		t.Fatal("OpsStr4SetJSON 期望错误")
	}
	if _, _, err := helper.OpsHash4Get(ctx, "k", "f"); err == nil {
		t.Fatal("OpsHash4Get 期望错误")
	}
	if _, _, err := helper.OpsHash4GetAll(ctx, "k"); err == nil {
		t.Fatal("OpsHash4GetAll 期望错误")
	}
	if err := helper.OpsHash4Set(ctx, "k", map[string]any{"f": "v"}, time.Minute); err == nil {
		t.Fatal("OpsHash4Set 期望错误")
	}
	if _, err := helper.OpsKey4Exists(ctx, "k"); err == nil {
		t.Fatal("OpsKey4Exists 期望错误")
	}
	if _, err := helper.OpsKey4Expire(ctx, "k", time.Minute); err == nil {
		t.Fatal("OpsKey4Expire 期望错误")
	}
	if err := helper.OpsKey4Delete(ctx, "k"); err == nil {
		t.Fatal("OpsKey4Delete 期望错误")
	}
	if err := helper.OpsSet4Add(ctx, "k", "m"); err == nil {
		t.Fatal("OpsSet4Add 期望错误")
	}
	if err := helper.OpsSet4Remove(ctx, "k", "m"); err == nil {
		t.Fatal("OpsSet4Remove 期望错误")
	}
	if _, err := helper.OpsSet4Members(ctx, "k"); err == nil {
		t.Fatal("OpsSet4Members 期望错误")
	}
	if _, err := helper.OpsScript4Eval(ctx, "return 1", nil); err == nil {
		t.Fatal("OpsScript4Eval 期望错误")
	}
	if _, err := helper.OpsRateLimit4Allow(ctx, "k", 10, 1, time.Minute); err == nil {
		t.Fatal("OpsRateLimit4Allow 期望错误")
	}
}
