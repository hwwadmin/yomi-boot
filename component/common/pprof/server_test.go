package pprof

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestPprofIndex 验证 /debug/pprof/ 路由可访问并返回内容
func TestPprofIndex(t *testing.T) {
	t.Parallel()

	srv := MustStart(Config{Addr: "127.0.0.1:0"})
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
