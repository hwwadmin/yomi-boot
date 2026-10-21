// Package ws 心跳空闲超时与 Ping 续命联调
//
// @author zwei
package ws

import (
	"sync"
	"testing"
	"time"
)

// TestHeartbeatIdleTimeout 服务端空闲超时应踢掉不发 Ping 的客户端
func TestHeartbeatIdleTimeout(t *testing.T) {
	t.Log("[用例] 服务端 HeartbeatTimeout 到期断开")

	cliH := newDisconnectClientHandler()
	srv, url := startTestServer(t, ServerConfig{
		Addr:              "127.0.0.1:0",
		HeartbeatInterval: Duration(0),
		HeartbeatTimeout:  Duration(200 * time.Millisecond),
		MaxConnLifetime:   Duration(0),
		Handler:           NopServerHandler{},
	})
	cli := connectTestClient(t, url, ClientConfig{
		HeartbeatInterval: Duration(0),
		HeartbeatTimeout:  Duration(0),
		Handler:           cliH,
	})

	waitUntil(t, testWait, func() bool { return srv.Len() == 1 }, "等待登记超时")
	cliH.wait(t)
	waitUntil(t, testWait, func() bool { return !cli.Connected() && srv.Len() == 0 }, "空闲超时后应断开")
	t.Log("[断言] 客户端 OnDisconnect，Hub 为空")
}

// TestHeartbeatPingKeepsAlive 客户端周期 Ping 应刷新服务端空闲定时器
func TestHeartbeatPingKeepsAlive(t *testing.T) {
	t.Log("[用例] 客户端 Ping 续命，超时窗口内不断开")

	cliH := newDisconnectClientHandler()
	srv, url := startTestServer(t, ServerConfig{
		Addr:              "127.0.0.1:0",
		HeartbeatInterval: Duration(0),
		HeartbeatTimeout:  Duration(300 * time.Millisecond),
		MaxConnLifetime:   Duration(0),
		Handler:           NopServerHandler{},
	})
	cli := connectTestClient(t, url, ClientConfig{
		HeartbeatInterval: Duration(80 * time.Millisecond),
		HeartbeatTimeout:  Duration(0),
		Handler:           cliH,
	})

	waitUntil(t, testWait, func() bool { return srv.Len() == 1 }, "等待登记超时")
	time.Sleep(700 * time.Millisecond)
	if !cli.Connected() || srv.Len() != 1 {
		t.Fatalf("Ping 续命失败 Connected=%v Len=%d", cli.Connected(), srv.Len())
	}
	select {
	case <-cliH.done:
		t.Fatal("超时窗口内不应 OnDisconnect")
	default:
	}
	t.Log("[断言] 700ms 后仍在线")
}

// disconnectClientHandler 记录客户端断开
type disconnectClientHandler struct {
	done   chan struct{}
	finish sync.Once
}

func newDisconnectClientHandler() *disconnectClientHandler {
	return &disconnectClientHandler{done: make(chan struct{})}
}

func (h *disconnectClientHandler) OnConnect(PeerContext) {}

func (h *disconnectClientHandler) OnDisconnect(error) {
	h.finish.Do(func() { close(h.done) })
}

func (h *disconnectClientHandler) OnPayload(PeerContext, Payload) {}

func (h *disconnectClientHandler) wait(t *testing.T) {
	t.Helper()
	waitDone(t, h.done, "等待客户端断开超时")
}
