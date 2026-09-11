// Package ws 通道最长存活配置与联调测试
//
// @author zwei
package ws

import (
	"testing"
	"time"
)

// TestMaxConnLifetimeKicks 到期应主动断开，流量不续命
func TestMaxConnLifetimeKicks(t *testing.T) {
	t.Log("[用例] MaxConnLifetime 到期踢连接")

	cliH := newDisconnectClientHandler()
	srv, url := startTestServer(t, ServerConfig{
		Addr:                  "127.0.0.1:0",
		HeartbeatInterval:     Duration(0),
		HeartbeatTimeout:      Duration(0),
		MaxConnLifetime:       Duration(200 * time.Millisecond),
		MaxConnLifetimeJitter: Duration(0),
		Handler:               NopServerHandler{},
	})
	cli := connectTestClient(t, url, ClientConfig{
		HeartbeatInterval: Duration(50 * time.Millisecond),
		HeartbeatTimeout:  Duration(0),
		Handler:           cliH,
	})

	waitUntil(t, testWait, func() bool { return srv.Len() == 1 }, "等待登记超时")
	cliH.wait(t)
	waitUntil(t, testWait, func() bool { return !cli.Connected() && srv.Len() == 0 }, "寿命到期后应断开")
	t.Log("[断言] Ping 不能续命，到期已断开")
}

func TestJitteredConnLifetimeRange(t *testing.T) {
	life := 10 * time.Second
	jitter := time.Second
	for i := 0; i < 32; i++ {
		got := jitteredConnLifetime(life, jitter)
		if got < life-jitter || got > life+jitter {
			t.Fatalf("jittered=%v 超出 [%v,%v]", got, life-jitter, life+jitter)
		}
	}
	if got := jitteredConnLifetime(life, 0); got != life {
		t.Fatalf("jitter=0 应得 %v，实际 %v", life, got)
	}
}
