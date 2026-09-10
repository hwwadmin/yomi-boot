// Package wsbase 通道最长存活配置与联调测试
//
// @author zwei
package wsbase

import (
	"context"
	"testing"
	"time"
)

// TestResolveMaxConnLifetime 覆盖 nil / 显式 0 / 自定义
func TestResolveMaxConnLifetime(t *testing.T) {
	t.Parallel()

	life, jit := resolveMaxConnLifetime(nil, nil)
	if life != DefaultMaxConnLifetime || jit != DefaultMaxConnLifetimeJitter {
		t.Fatalf("nil/nil => %v/%v, want %v/%v", life, jit, DefaultMaxConnLifetime, DefaultMaxConnLifetimeJitter)
	}

	life, jit = resolveMaxConnLifetime(Duration(0), Duration(0))
	if life != 0 || jit != 0 {
		t.Fatalf("0/0 => %v/%v, want 0/0", life, jit)
	}

	life, jit = resolveMaxConnLifetime(Duration(10*time.Second), Duration(0))
	if life != 10*time.Second || jit != 0 {
		t.Fatalf("10s/0 => %v/%v", life, jit)
	}

	life, jit = resolveMaxConnLifetime(Duration(0), Duration(30*time.Second))
	if life != 0 || jit != 30*time.Second {
		t.Fatalf("0/30s => %v/%v", life, jit)
	}

	cfg := ServerConfig{Addr: ":0"}
	life, jit = cfg.ResolvedMaxConnLifetime()
	if life != DefaultMaxConnLifetime || jit != DefaultMaxConnLifetimeJitter {
		t.Fatalf("Resolved 默认 => %v/%v", life, jit)
	}
}

// TestValidateMaxConnLifetime 覆盖负数、Jitter>=Lifetime、不限制时忽略默认 jitter
func TestValidateMaxConnLifetime(t *testing.T) {
	t.Parallel()

	if err := validateMaxConnLifetime(Duration(-time.Second), nil); err == nil {
		t.Fatal("负 Lifetime 应失败")
	}
	if err := validateMaxConnLifetime(nil, Duration(-time.Second)); err == nil {
		t.Fatal("负 Jitter 应失败")
	}
	if err := validateMaxConnLifetime(Duration(10*time.Second), nil); err == nil {
		t.Fatal("10s 配默认 30s jitter 应失败")
	}
	if err := validateMaxConnLifetime(Duration(10*time.Second), Duration(10*time.Second)); err == nil {
		t.Fatal("Jitter==Lifetime 应失败")
	}
	if err := validateMaxConnLifetime(Duration(10*time.Second), Duration(20*time.Second)); err == nil {
		t.Fatal("Jitter>Lifetime 应失败")
	}
	if err := validateMaxConnLifetime(nil, nil); err != nil {
		t.Fatalf("默认应通过: %v", err)
	}
	if err := validateMaxConnLifetime(Duration(0), nil); err != nil {
		t.Fatalf("不限制应忽略默认 jitter: %v", err)
	}
	if err := validateMaxConnLifetime(Duration(0), Duration(0)); err != nil {
		t.Fatalf("双 0 应通过: %v", err)
	}
	if err := validateMaxConnLifetime(Duration(24*time.Hour), Duration(30*time.Second)); err != nil {
		t.Fatalf("24h±30s 应通过: %v", err)
	}
	if err := (ServerConfig{Addr: ":0"}).Validate(); err != nil {
		t.Fatalf("仅 Addr 应通过: %v", err)
	}
	if err := (ServerConfig{Addr: ":0", MaxConnLifetime: Duration(10 * time.Second)}).Validate(); err == nil {
		t.Fatal("短 Lifetime 配默认 jitter 的 ServerConfig 应失败")
	}
}

// TestJitteredConnLifetime 覆盖准时与闭区间抖动
func TestJitteredConnLifetime(t *testing.T) {
	t.Parallel()

	if got := jitteredConnLifetime(time.Second, 0); got != time.Second {
		t.Fatalf("jitter=0 => %v, want 1s", got)
	}

	lifetime := 10 * time.Second
	jitter := 2 * time.Second
	min := lifetime - jitter
	max := lifetime + jitter
	seenOther := false
	for i := 0; i < 32; i++ {
		got := jitteredConnLifetime(lifetime, jitter)
		if got < min || got > max {
			t.Fatalf("第 %d 次 %v 超出 [%v, %v]", i, got, min, max)
		}
		if got != lifetime {
			seenOther = true
		}
	}
	if !seenOther {
		t.Fatal("32 次抽样全等于基准，抖动未生效")
	}
}

// TestMaxConnLifetimeKicksDespiteTraffic 持续发消息仍应在寿命到期后断开
func TestMaxConnLifetimeKicksDespiteTraffic(t *testing.T) {
	zero := Duration(0)
	short := Duration(200 * time.Millisecond)
	env := startServerAndClient(t,
		ServerConfig{
			HeartbeatInterval:     zero,
			HeartbeatTimeout:      zero,
			MaxConnLifetime:       short,
			MaxConnLifetimeJitter: zero,
		},
		ClientConfig{
			HeartbeatInterval: zero,
			HeartbeatTimeout:  zero,
		},
	)
	defer env.close()

	stopWrite := make(chan struct{})
	go func() {
		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopWrite:
				return
			case <-ticker.C:
				_ = env.clientConn.WriteText(context.Background(), []byte("keep"))
			}
		}
	}()
	defer close(stopWrite)

	select {
	case <-env.serverClose:
		t.Log("[断言] 有流量时服务端仍因最长存活断开")
	case <-time.After(2 * time.Second):
		t.Fatal("等待服务端最长存活断开超时")
	}
}

// TestMaxConnLifetimeZeroKeepsAlive 显式 0 不限制时应保持连接
func TestMaxConnLifetimeZeroKeepsAlive(t *testing.T) {
	zero := Duration(0)
	env := startServerAndClient(t,
		ServerConfig{
			HeartbeatInterval:     zero,
			HeartbeatTimeout:      zero,
			MaxConnLifetime:       zero,
			MaxConnLifetimeJitter: zero,
		},
		ClientConfig{
			HeartbeatInterval: zero,
			HeartbeatTimeout:  zero,
		},
	)
	defer env.close()

	select {
	case <-env.serverClose:
		t.Fatal("MaxConnLifetime=0 时服务端不应过早关闭")
	case <-env.clientClose:
		t.Fatal("MaxConnLifetime=0 时客户端不应过早关闭")
	case <-time.After(600 * time.Millisecond):
		t.Log("[断言] 超过短寿命窗口仍存活")
	}

	if err := env.clientConn.WriteText(context.Background(), []byte("still-alive")); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	got := env.waitServerMsg()
	if string(got) != "still-alive" {
		t.Fatalf("got %q", got)
	}
}

// TestMaxConnLifetimeKicksIdle 心跳全关且无业务流量时仍应按寿命断开
func TestMaxConnLifetimeKicksIdle(t *testing.T) {
	zero := Duration(0)
	short := Duration(200 * time.Millisecond)
	env := startServerAndClient(t,
		ServerConfig{
			HeartbeatInterval:     zero,
			HeartbeatTimeout:      zero,
			MaxConnLifetime:       short,
			MaxConnLifetimeJitter: zero,
		},
		ClientConfig{
			HeartbeatInterval: zero,
			HeartbeatTimeout:  zero,
		},
	)
	defer env.close()

	select {
	case <-env.serverClose:
		t.Log("[断言] 无流量时服务端因最长存活断开")
	case <-time.After(2 * time.Second):
		t.Fatal("等待服务端最长存活断开超时")
	}
}
