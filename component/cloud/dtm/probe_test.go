// DTM 启动探活的单元测试

package dtm

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestProbeServerEmptyNoop 验证空地址直接返回，不发起网络请求
func TestProbeServerEmptyNoop(t *testing.T) {
	ProbeServer("")
	ProbeServer("   ")
}

// TestProbeServerHitsVersion 验证探测请求打到 DTM 服务端的 /version 路径
func TestProbeServerHitsVersion(t *testing.T) {
	var hits atomic.Int32
	var gotPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotPath.Store(r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"1.0"}`))
	}))
	defer srv.Close()

	ProbeServer(srv.URL)

	if hits.Load() != 1 {
		t.Fatalf("应探测 1 次，实际 %d", hits.Load())
	}
	if gotPath.Load().(string) != "/version" {
		t.Fatalf("探测路径期望 /version，实际 %v", gotPath.Load())
	}
}

// TestProbeServerNon2xx 验证非 2xx 响应走失败告警分支且不重试
func TestProbeServerNon2xx(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ProbeServer(srv.URL)

	if hits.Load() != 1 {
		t.Fatalf("应探测 1 次，实际 %d", hits.Load())
	}
}

// TestProbeServerConnectionRefused 验证服务端不可达时走失败告警分支
func TestProbeServerConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // 先关闭监听，保证连接被拒绝

	ProbeServer(url)
}

// TestProbeServerInvalidURL 验证非法地址在构造请求时即失败
func TestProbeServerInvalidURL(t *testing.T) {
	ProbeServer("http://exa mple.com") // 含空格，无法构造合法请求
}
