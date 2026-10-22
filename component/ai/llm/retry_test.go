// max token 字段 400 回退单测。
//
// @author zwei

package llm

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"gitee.com/hq_project/yomi-boot/core/errors"
)

// maxTokensFieldShim 第一次返回 400 拒绝 max_tokens，第二次接受 max_completion_tokens
type maxTokensFieldShim struct {
	mu     sync.Mutex
	bodies []string
	stream bool
}

// RoundTrip 按请求体中的 max token 字段返回 400 或成功响应
func (s *maxTokensFieldShim) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	s.mu.Lock()
	s.bodies = append(s.bodies, string(body))
	s.mu.Unlock()
	if strings.Contains(string(body), `"max_tokens"`) {
		return jsonResponse(req, http.StatusBadRequest, `{"error":{"message":"Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.","type":"invalid_request_error","param":"max_tokens","code":"unsupported_parameter"}}`), nil
	}
	if s.stream {
		return sseResponse(req), nil
	}
	return jsonResponse(req, http.StatusOK, `{"id":"1","object":"chat.completion","created":0,"model":"gateway-alias","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`), nil
}

// jsonResponse 构造 JSON 响应
func jsonResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Request:    req,
	}
}

// sseResponse 构造单增量 SSE 响应
func sseResponse(req *http.Request) *http.Response {
	body := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"gateway-alias\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Request:    req,
	}
}

// failMsgShim 固定返回 400 错误正文，用于验证启发式误判
type failMsgShim struct {
	mu    sync.Mutex
	calls int
	msg   string
}

// RoundTrip 返回带指定 message 的 400
func (s *failMsgShim) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.calls++
	msg := s.msg
	s.mu.Unlock()
	return jsonResponse(req, http.StatusBadRequest, `{"error":{"message":"`+msg+`"}}`), nil
}

// newRetryClient 构造面向别名网关的客户端
func newRetryClient(t *testing.T, shim *maxTokensFieldShim) Client {
	t.Helper()
	cli, err := New(Config{
		Protocol:   ProtocolOpenAI,
		APIURL:     "https://gateway.example/v1",
		APIKey:     "sk-test",
		Model:      "gateway-alias",
		HTTPClient: &http.Client{Transport: shim},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return cli
}

// TestChatRetriesWithAlternateMaxTokensField 验证 Chat 在 400 后切换字段并成功
func TestChatRetriesWithAlternateMaxTokensField(t *testing.T) {
	t.Parallel()
	shim := &maxTokensFieldShim{}
	cli := newRetryClient(t, shim)
	resp, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Message.Content != "hi" {
		t.Fatalf("resp=%+v", resp)
	}
	shim.mu.Lock()
	defer shim.mu.Unlock()
	if len(shim.bodies) != 2 {
		t.Fatalf("calls=%d", len(shim.bodies))
	}
	if !strings.Contains(shim.bodies[0], `"max_tokens"`) {
		t.Fatalf("first body=%s", shim.bodies[0])
	}
	if strings.Contains(shim.bodies[1], `"max_tokens"`) || !strings.Contains(shim.bodies[1], `"max_completion_tokens"`) {
		t.Fatalf("second body=%s", shim.bodies[1])
	}
}

// TestChatDoesNotRetryOnMaxTokensValueError 验证 max_tokens 值错误不会触发字段切换重试
func TestChatDoesNotRetryOnMaxTokensValueError(t *testing.T) {
	t.Parallel()
	shim := &failMsgShim{msg: "max_tokens is too large"}
	cli, err := New(Config{
		Protocol:   ProtocolOpenAI,
		APIURL:     "https://gateway.example/v1",
		APIKey:     "sk-test",
		Model:      "gateway-alias",
		HTTPClient: &http.Client{Transport: shim},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err=%v", err)
	}
	shim.mu.Lock()
	defer shim.mu.Unlock()
	if shim.calls != 1 {
		t.Fatalf("calls=%d", shim.calls)
	}
}

// TestChatStreamRetriesWithAlternateMaxTokensField 验证流式建连 400 后切换字段并成功
func TestChatStreamRetriesWithAlternateMaxTokensField(t *testing.T) {
	t.Parallel()
	shim := &maxTokensFieldShim{stream: true}
	cli := newRetryClient(t, shim)
	stream, err := cli.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() { _ = stream.Close() }()
	chunk, err := stream.Recv()
	if err != nil || chunk.Delta != "hi" {
		t.Fatalf("chunk=%+v err=%v", chunk, err)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("second Recv: %v", err)
	}
	shim.mu.Lock()
	defer shim.mu.Unlock()
	if len(shim.bodies) != 2 {
		t.Fatalf("calls=%d", len(shim.bodies))
	}
}
