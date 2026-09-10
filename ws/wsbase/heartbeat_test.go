// Package wsbase 心跳配置与联调测试
//
// @author zwei
package wsbase

import (
	"context"
	"testing"
	"time"
)

// TestResolveHeartbeat 覆盖 nil / 显式 0 / 自定义
func TestResolveHeartbeat(t *testing.T) {
	t.Parallel()

	i, to := resolveHeartbeat(nil, nil)
	if i != DefaultHeartbeatInterval || to != DefaultHeartbeatTimeout {
		t.Fatalf("nil/nil => %v/%v, want %v/%v", i, to, DefaultHeartbeatInterval, DefaultHeartbeatTimeout)
	}

	i, to = resolveHeartbeat(Duration(0), Duration(0))
	if i != 0 || to != 0 {
		t.Fatalf("0/0 => %v/%v, want 0/0", i, to)
	}

	i, to = resolveHeartbeat(Duration(5*time.Second), Duration(0))
	if i != 5*time.Second || to != 0 {
		t.Fatalf("5s/0 => %v/%v", i, to)
	}

	i, to = resolveHeartbeat(Duration(0), Duration(20*time.Second))
	if i != 0 || to != 20*time.Second {
		t.Fatalf("0/20s => %v/%v", i, to)
	}
}

// TestValidateHeartbeat 覆盖负数与 Timeout<=Interval
func TestValidateHeartbeat(t *testing.T) {
	t.Parallel()

	if err := validateHeartbeat(Duration(-time.Second), nil); err == nil {
		t.Fatal("负 Interval 应失败")
	}
	if err := validateHeartbeat(nil, Duration(-time.Second)); err == nil {
		t.Fatal("负 Timeout 应失败")
	}
	if err := validateHeartbeat(Duration(10*time.Second), Duration(10*time.Second)); err == nil {
		t.Fatal("Timeout==Interval 应失败")
	}
	if err := validateHeartbeat(Duration(10*time.Second), Duration(5*time.Second)); err == nil {
		t.Fatal("Timeout<Interval 应失败")
	}
	if err := validateHeartbeat(nil, nil); err != nil {
		t.Fatalf("默认应通过: %v", err)
	}
	if err := validateHeartbeat(Duration(0), Duration(0)); err != nil {
		t.Fatalf("双 0 应通过: %v", err)
	}
	if err := validateHeartbeat(Duration(10*time.Second), Duration(0)); err != nil {
		t.Fatalf("仅 Ping 应通过: %v", err)
	}
	if err := (ServerConfig{Addr: ":0", HeartbeatInterval: Duration(10 * time.Second), HeartbeatTimeout: Duration(30 * time.Second)}).Validate(); err != nil {
		t.Fatal(err)
	}
}

// TestHeartbeatTimeoutKicksIdle 无 Ping、短超时、无业务流量时应断开
func TestHeartbeatTimeoutKicksIdle(t *testing.T) {
	zero := Duration(0)
	short := Duration(200 * time.Millisecond)
	env := startServerAndClient(t,
		ServerConfig{
			HeartbeatInterval: zero,
			HeartbeatTimeout:  short,
		},
		ClientConfig{
			HeartbeatInterval: zero,
			HeartbeatTimeout:  zero,
		},
	)
	defer env.close()

	select {
	case <-env.serverClose:
		t.Log("[断言] 服务端因心跳超时关闭")
	case <-time.After(2 * time.Second):
		t.Fatal("等待服务端心跳超时关闭超时")
	}
}

// TestHeartbeatPingKeepsAlive 短超时下周期 Ping 应维持连接
func TestHeartbeatPingKeepsAlive(t *testing.T) {
	interval := Duration(50 * time.Millisecond)
	timeout := Duration(200 * time.Millisecond)
	env := startServerAndClient(t,
		ServerConfig{
			HeartbeatInterval: interval,
			HeartbeatTimeout:  timeout,
		},
		ClientConfig{
			HeartbeatInterval: Duration(0),
			HeartbeatTimeout:  Duration(0),
		},
	)
	defer env.close()

	select {
	case <-env.serverClose:
		t.Fatal("有 Ping 保活时服务端不应过早关闭")
	case <-env.clientClose:
		t.Fatal("有 Ping 保活时客户端不应过早关闭")
	case <-time.After(600 * time.Millisecond):
		t.Log("[断言] 空闲超过 Timeout 仍存活，Ping 保活生效")
	}

	// 业务读写仍可用
	if err := env.clientConn.WriteText(context.Background(), []byte("still-alive")); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	got := env.waitServerMsg()
	if string(got) != "still-alive" {
		t.Fatalf("got %q", got)
	}
}
