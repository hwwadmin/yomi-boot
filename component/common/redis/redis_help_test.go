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
