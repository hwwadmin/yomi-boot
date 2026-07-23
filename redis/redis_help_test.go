package redis

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/redis/redistest"
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

// TestNewRedisHelper_NilClientPanics 验证未初始化客户端时构造助手会 panic
func TestNewRedisHelper_NilClientPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("期望 panic")
		}
	}()
	NewRedisHelper(nil)
}
