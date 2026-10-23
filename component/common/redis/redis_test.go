package redis

import (
	"context"
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
}
