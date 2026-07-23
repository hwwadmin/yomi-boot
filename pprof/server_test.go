package pprof

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestMustStartDisabled 验证 enabled=false 时 MustStart 不启动服务
func TestMustStartDisabled(t *testing.T) {
	t.Parallel()
	MustStart(Config{Enabled: boolPtr(false)})
}

// TestMustStartDefaultDisabled 验证未配置 enabled 时 MustStart 不启动服务
func TestMustStartDefaultDisabled(t *testing.T) {
	t.Parallel()
	MustStart(Config{})
}

// TestPprofIndex 验证 /debug/pprof/ 路由可访问并返回内容
func TestPprofIndex(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer func() { _ = listener.Close() }()

	mux := http.NewServeMux()
	registerRoutes(mux)
	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()

	deadline := time.Now().Add(3 * time.Second)
	var resp *http.Response
	for time.Now().Before(deadline) {
		resp, err = http.Get("http://" + listener.Addr().String() + "/debug/pprof/")
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
