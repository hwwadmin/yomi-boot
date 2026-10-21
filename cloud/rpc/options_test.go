// @author zwei

package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// TestNewHttpRpcClient_DefaultTransport 验证默认传输和重定向策略
func TestNewHttpRpcClient_DefaultTransport(t *testing.T) {
	client := NewHttpRpcClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example.test", nil
	}), "svc")
	if client.httpClient.Transport != defaultTransport {
		t.Fatalf("默认应使用共享 transport，实际 %T", client.httpClient.Transport)
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("默认 transport 应为 *http.Transport，实际 %T", client.httpClient.Transport)
	}
	if any(transport) == http.DefaultTransport {
		t.Fatal("不应直接使用全局 http.DefaultTransport")
	}
	if transport.MaxIdleConns != defaultMaxIdleConns {
		t.Fatalf("MaxIdleConns: got %d want %d", transport.MaxIdleConns, defaultMaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != defaultMaxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost: got %d want %d", transport.MaxIdleConnsPerHost, defaultMaxIdleConnsPerHost)
	}
	if client.httpClient.CheckRedirect == nil {
		t.Fatal("默认应设置重定向策略")
	}
}

// TestNewHttpRpcClient_WithTransport 验证可以注入自定义传输
func TestNewHttpRpcClient_WithTransport(t *testing.T) {
	var reads, bytesRead atomic.Int64
	transport := &fakeTransport{
		status: http.StatusOK, body: []byte(`{"code":"OK"}`),
		reads: &reads, bytesRead: &bytesRead,
	}
	client := NewHttpRpcClient(resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example.test/x", nil
	}), "svc", WithTransport(transport))
	if client.httpClient.Transport != transport {
		t.Fatalf("应使用注入的 transport，实际 %T", client.httpClient.Transport)
	}
	if err := invokeTest(client, context.Background(), "/x", nil, nil, NoopBizHook); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if reads.Load() == 0 {
		t.Fatal("应通过注入的 transport 发起请求")
	}
}

// TestInvoke_DefaultDoesNotFollowRedirect 验证默认不跟随重定向
func TestInvoke_DefaultDoesNotFollowRedirect(t *testing.T) {
	var redirected atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/other" {
			w.Header().Set("Location", "/other")
			w.WriteHeader(http.StatusFound)
			return
		}
		redirected.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL)
	var env testEnvelope
	err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope)
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusFound {
		t.Fatalf("期望 302 HTTPStatusError，实际 %v", err)
	}
	if redirected.Load() != 0 {
		t.Fatalf("默认不应跟随重定向，/other 命中 %d 次", redirected.Load())
	}
}

// TestInvoke_WithCheckRedirectFollows 验证可以覆盖默认重定向策略
func TestInvoke_WithCheckRedirectFollows(t *testing.T) {
	var redirected atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/other" {
			w.Header().Set("Location", "/other")
			w.WriteHeader(http.StatusFound)
			return
		}
		redirected.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL, WithCheckRedirect(func(*http.Request, []*http.Request) error {
		return nil
	}))
	var env testEnvelope
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); err != nil {
		t.Fatalf("允许跟随重定向后应成功: %v", err)
	}
	if redirected.Load() != 1 {
		t.Fatalf("应跟随一次重定向，/other 命中 %d 次", redirected.Load())
	}
}
