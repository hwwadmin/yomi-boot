package redis

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// TestNewClient_AppliesCommandTimeout 验证指令超时同时落到客户端读写超时与 context 超时开关
func TestNewClient_AppliesCommandTimeout(t *testing.T) {
	mr := miniredis.RunT(t)

	client, err := NewClient(context.Background(), Config{Addr: mr.Addr(), CommandTimeoutMs: 1500})
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	defer func() {
		_ = client.Close()
	}()

	opts := client.Options()
	if opts.ReadTimeout != 1500*time.Millisecond {
		t.Fatalf("ReadTimeout=%v want 1500ms", opts.ReadTimeout)
	}
	if opts.WriteTimeout != 1500*time.Millisecond {
		t.Fatalf("WriteTimeout=%v want 1500ms", opts.WriteTimeout)
	}
	if !opts.ContextTimeoutEnabled {
		t.Fatal("ContextTimeoutEnabled 应为 true，否则 context 超时不生效")
	}
	if opts.MaxRetries != 0 {
		t.Fatalf("MaxRetries=%d want 0，驱动把 -1 收成不再重试", opts.MaxRetries)
	}
}

// TestNewClient_TrimsAddr 验证地址两端空白不影响连接
func TestNewClient_TrimsAddr(t *testing.T) {
	mr := miniredis.RunT(t)

	client, err := NewClient(context.Background(), Config{Addr: " " + mr.Addr() + " ", CommandTimeoutMs: 1500})
	if err != nil {
		t.Fatalf("两端有空白的地址应能连上: %v", err)
	}
	defer func() {
		_ = client.Close()
	}()
	if client.Options().Addr != mr.Addr() {
		t.Fatalf("Addr=%q want %q", client.Options().Addr, mr.Addr())
	}
}

// TestNewClient_RejectsEmptyAddrAndNegativeDB 验证空地址和负数库编号在拨号前返回错误
func TestNewClient_RejectsEmptyAddrAndNegativeDB(t *testing.T) {
	ctx := context.Background()

	_, err := NewClient(ctx, Config{CommandTimeoutMs: 200})
	if err == nil || !strings.Contains(err.Error(), "addr 不能为空") {
		t.Fatalf("空地址应在拨号前返回错误，实际 %v", err)
	}

	_, err = NewClient(ctx, Config{Addr: "127.0.0.1:1", DB: -1, CommandTimeoutMs: 200})
	if err == nil || !strings.Contains(err.Error(), "db 不能小于 0") {
		t.Fatalf("负数库编号应在拨号前返回错误，实际 %v", err)
	}
}
