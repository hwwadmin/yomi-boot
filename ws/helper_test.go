// Package ws 测试辅助：启动独立端口 Server/Client
//
// @author zwei
package ws

import (
	"context"
	"testing"
	"time"
)

// startTestServer 启动明文监听并在 Cleanup 时关闭
func startTestServer(t *testing.T, cfg ServerConfig) (Server, string) {
	t.Helper()
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv, err := NewServer(cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	addr, err := srv.ListenAddr()
	if err != nil {
		cancel()
		_ = srv.Close(context.Background())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = srv.Close(context.Background())
	})
	return srv, "ws://" + addr.String() + "/"
}

// connectTestClient 拨号并在 Cleanup 时关闭
func connectTestClient(t *testing.T, url string, cfg ClientConfig) Client {
	t.Helper()
	cli, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.Connect(context.Background(), url); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cli.Close(CloseNormalClosure, "test done")
	})
	return cli
}

// waitUntil 在超时前轮询 cond
func waitUntil(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}
