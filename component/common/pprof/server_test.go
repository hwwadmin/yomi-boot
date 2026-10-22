package pprof

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestMustStartDisabled 验证 enabled=false 时 MustStart 返回 nil
func TestMustStartDisabled(t *testing.T) {
	t.Parallel()
	if srv := MustStart(Config{Enabled: boolPtr(false)}); srv != nil {
		t.Fatal("enabled=false 时 MustStart 应返回 nil")
	}
}

// TestMustStartDefaultDisabled 验证未配置 enabled 时 MustStart 返回 nil
func TestMustStartDefaultDisabled(t *testing.T) {
	t.Parallel()
	if srv := MustStart(Config{}); srv != nil {
		t.Fatal("未配置 enabled 时 MustStart 应返回 nil")
	}
}

// TestPprofIndex 验证 /debug/pprof/ 路由可访问并返回内容
func TestPprofIndex(t *testing.T) {
	t.Parallel()

	srv := MustStart(Config{Enabled: boolPtr(true), Addr: "127.0.0.1:0"})
	if srv == nil {
		t.Fatal("MustStart 返回 nil")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})

	addr := srv.Addr()
	if addr == nil {
		t.Fatal("Addr 返回 nil")
	}

	deadline := time.Now().Add(3 * time.Second)
	var resp *http.Response
	var err error
	for time.Now().Before(deadline) {
		resp, err = http.Get("http://" + addr.String() + "/debug/pprof/")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码不符合预期: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("响应体不能为空")
	}
}

// boolPtr 返回 bool 指针辅助函数
func boolPtr(v bool) *bool {
	return &v
}
