// Package redis Fail Fast 初始化测试
//
// @author zwei
package redis

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
)

// TestMustNewClient_MiniRedis 验证合法配置创建可用客户端
func TestMustNewClient_MiniRedis(t *testing.T) {
	addr, cleanup := redistest.NewServer(t)
	defer cleanup()

	client := MustNewClient(context.Background(), Config{Addr: addr})
	if client == nil {
		t.Fatal("MustNewClient 应返回可用客户端")
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("客户端应可 Ping: %v", err)
	}
	_ = client.Close()
}

// TestMustNewClient_PanicsOnInvalidConfig 验证配置非法时 panic
func TestMustNewClient_PanicsOnInvalidConfig(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("配置非法应 panic")
		}
	}()
	MustNewClient(context.Background(), Config{})
}

// TestNewClient_RejectsUnreachableAddr 验证连接不可达时返回错误
func TestNewClient_RejectsUnreachableAddr(t *testing.T) {
	// 使用必然无人监听的端口触发连接验证失败
	_, err := NewClient(context.Background(), Config{Addr: "127.0.0.1:1", CommandTimeoutMs: 300})
	if err == nil {
		t.Fatal("连接不可达应返回错误")
	}
}

// TestMustNewClient_PanicsOnUnreachable 验证连接验证失败时 Fail Fast
func TestMustNewClient_PanicsOnUnreachable(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("连接验证失败应 panic")
		}
	}()
	MustNewClient(context.Background(), Config{Addr: "127.0.0.1:1", CommandTimeoutMs: 300})
}
