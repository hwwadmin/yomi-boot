package rpc

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// countingBody 统计响应体读取次数和字节数
type countingBody struct {
	data      []byte
	off       int
	reads     *atomic.Int64
	bytesRead *atomic.Int64
}

// Read 记录读取次数和字节数
func (b *countingBody) Read(p []byte) (int, error) {
	b.reads.Add(1)
	if b.off >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.off:])
	b.off += n
	b.bytesRead.Add(int64(n))
	return n, nil
}

// Close 关闭响应体
func (b *countingBody) Close() error { return nil }

// fakeTransport 返回固定状态码和响应体
type fakeTransport struct {
	status    int
	body      []byte
	reads     *atomic.Int64
	bytesRead *atomic.Int64
}

// RoundTrip 实现 http.RoundTripper
func (t *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    t.status,
		Status:        http.StatusText(t.status),
		Header:        make(http.Header),
		Body:          &countingBody{data: t.body, reads: t.reads, bytesRead: t.bytesRead},
		Request:       req,
		ContentLength: int64(len(t.body)),
	}, nil
}

// TestInvoke_VoidDrainsBody 验证 void 调用排空小 body 以复用连接
func TestInvoke_VoidDrainsBody(t *testing.T) {
	var reads, bytesRead atomic.Int64
	body := []byte(`{"code":"OK"}`)
	client := newTestRPCClient("")
	client.httpClient = &http.Client{Transport: &fakeTransport{
		status: http.StatusOK, body: body,
		reads: &reads, bytesRead: &bytesRead,
	}}
	client.resolver = resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example.test/x", nil
	})

	if err := invokeTest(client, context.Background(), "/x", nil, nil, NoopBizHook); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if bytesRead.Load() != int64(len(body)) || reads.Load() == 0 {
		t.Fatalf("void 调用应排空小 body，reads=%d bytes=%d", reads.Load(), bytesRead.Load())
	}
}

// TestInvoke_VoidNon200DrainsBody 验证非 200 也只排空有限 body
func TestInvoke_VoidNon200DrainsBody(t *testing.T) {
	var reads, bytesRead atomic.Int64
	client := newTestRPCClient("")
	client.httpClient = &http.Client{Transport: &fakeTransport{
		status: http.StatusInternalServerError, body: bytes.Repeat([]byte("x"), 10000),
		reads: &reads, bytesRead: &bytesRead,
	}}
	client.resolver = resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example.test/x", nil
	})

	err := invokeTest(client, context.Background(), "/x", nil, nil, NoopBizHook)
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusInternalServerError {
		t.Fatalf("期望 HTTP 500，实际 %v", err)
	}
	if bytesRead.Load() == 0 || bytesRead.Load() > int64(maxDrainBytes+1) {
		t.Fatalf("非 200 void 调用应只排空有限 body，reads=%d bytes=%d", reads.Load(), bytesRead.Load())
	}
}

// TestInvoke_BodyReadWhenNeeded 验证需要响应体时仍正常读取
func TestInvoke_BodyReadWhenNeeded(t *testing.T) {
	var reads, bytesRead atomic.Int64
	client := newTestRPCClient("")
	client.httpClient = &http.Client{Transport: &fakeTransport{
		status: http.StatusOK, body: []byte(`{"code":"OK"}`),
		reads: &reads, bytesRead: &bytesRead,
	}}
	client.resolver = resolverFunc(func(context.Context, string, string) (string, error) {
		return "http://example.test/x", nil
	})

	var env testEnvelope
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if reads.Load() == 0 || bytesRead.Load() == 0 {
		t.Fatalf("需要响应体时应读取 body，reads=%d bytes=%d", reads.Load(), bytesRead.Load())
	}
}

// TestInvoke_RequestMaxResponseBytesOverride 验证按调用覆盖响应体上限
func TestInvoke_RequestMaxResponseBytesOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL)
	var env testEnvelope
	if err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint:         "/x",
		RespBody:         &env,
		BizHook:          checkTestEnvelope,
		MaxResponseBytes: 1024,
	}); err != nil {
		t.Fatalf("请求级上限应覆盖客户端默认值: %v", err)
	}

	err := client.Invoke(context.Background(), InvokeRequest{
		Endpoint:         "/x",
		RespBody:         &env,
		BizHook:          checkTestEnvelope,
		MaxResponseBytes: 4,
	})
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("超过请求级上限应返回 ProtocolError，实际 %v", err)
	}
}
