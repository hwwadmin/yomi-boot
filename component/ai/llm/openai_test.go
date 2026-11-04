// OpenAI 协议适配单测。
//
// @author zwei

package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/errors"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
)

// openAIRequest 是测试侧解析的请求体
type openAIRequest struct {
	Model               string  `json:"model"`
	Temperature         float64 `json:"temperature"`
	MaxTokens           int     `json:"max_tokens"`
	MaxCompletionTokens int     `json:"max_completion_tokens"`
	Stream              bool    `json:"stream"`
	Messages            []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// newOpenAIClientForTest 构造指向测试服务器的 OpenAI 客户端
func newOpenAIClientForTest(t *testing.T, srv *httptest.Server, mutate func(*Config)) *openaiClient {
	t.Helper()
	cfg := Config{
		Protocol: ProtocolOpenAI,
		APIURL:   srv.URL + "/v1",
		APIKey:   "sk-test",
		Model:    "gpt-4o",
	}
	if mutate != nil {
		mutate(&cfg)
	}
	cli, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl, ok := cli.(*openaiClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	return impl
}

// writeOpenAIJSON 写 JSON 响应
func writeOpenAIJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// writeOpenAISSE 写 SSE 响应
func writeOpenAISSE(w http.ResponseWriter, frames ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	for _, f := range frames {
		_, _ = io.WriteString(w, "data: "+f+"\n\n")
	}
}

// openAIChatBody 是同步 Chat 的成功响应体
const openAIChatBody = `{"id":"chat_1","object":"chat.completion","created":0,"model":"gpt-4o",
"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`

// TestOpenAIChatMapsRequestAndResponse 验证同步对话的请求映射、头部与响应结构
func TestOpenAIChatMapsRequestAndResponse(t *testing.T) {
	t.Parallel()
	var got openAIRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("method=%s path=%s", r.Method, r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-test" {
			t.Errorf("authorization=%q", auth)
		}
		if v := r.Header.Get("x-gw"); v != "gw-1" {
			t.Errorf("x-gw=%q", v)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeOpenAIJSON(w, http.StatusOK, openAIChatBody)
	}))
	t.Cleanup(srv.Close)

	temp := 0.3
	maxTokens := 64
	cli := newOpenAIClientForTest(t, srv, func(c *Config) {
		c.ExtraHeaders = map[string]string{"x-gw": "gw-1"}
		c.HTTPClient = srv.Client()
		c.Timeout = 5 * time.Second
	})
	resp, err := cli.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: "s"},
			{Role: RoleUser, Content: "u"},
			{Role: RoleAssistant, Content: "a"},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.ID != "chat_1" || resp.Model != "gpt-4o" || resp.Message.Content != "hi" || resp.Message.Role != RoleAssistant {
		t.Fatalf("resp=%+v", resp)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 2 || resp.Usage.CompletionTokens != 3 || resp.Usage.TotalTokens != 5 {
		t.Fatalf("usage=%+v", resp.Usage)
	}
	if got.Model != "gpt-4o" || got.Temperature != 0.3 || got.MaxTokens != 64 || got.Stream {
		t.Fatalf("body=%+v", got)
	}
	if len(got.Messages) != 3 || got.Messages[0].Role != "system" || got.Messages[1].Role != "user" || got.Messages[2].Role != "assistant" {
		t.Fatalf("messages=%+v", got.Messages)
	}
}

// TestOpenAIChatDefaults 验证请求未给采样参数时回退配置缺省
func TestOpenAIChatDefaults(t *testing.T) {
	t.Parallel()
	var got openAIRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writeOpenAIJSON(w, http.StatusOK, `{"id":"c","object":"chat.completion","created":0,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{}}`)
	}))
	t.Cleanup(srv.Close)
	temp := 0.6
	maxTokens := 32
	cli := newOpenAIClientForTest(t, srv, func(c *Config) {
		c.Temperature = &temp
		c.MaxTokens = &maxTokens
	})
	resp, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Usage != nil {
		t.Fatalf("zero usage should be nil: %+v", resp.Usage)
	}
	if got.Temperature != 0.6 || got.MaxTokens != 32 {
		t.Fatalf("body=%+v", got)
	}
}

// TestOpenAIChatBadResponses 验证空 choices、空文本与空响应均为协议异常
func TestOpenAIChatBadResponses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{"null response", `null`},
		{"empty choices", `{"id":"c","object":"chat.completion","created":0,"model":"gpt-4o","choices":[],"usage":{}}`},
		{"empty content", `{"id":"c","object":"chat.completion","created":0,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeOpenAIJSON(w, http.StatusOK, tc.body)
			}))
			t.Cleanup(srv.Close)
			cli := newOpenAIClientForTest(t, srv, nil)
			resp, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
			if !errors.Is(err, ErrBadResponse) || resp != nil {
				t.Fatalf("resp=%+v err=%v", resp, err)
			}
		})
	}
}

// TestOpenAIChatErrorStatuses 验证 HTTP 状态到哨兵错误的映射
func TestOpenAIChatErrorStatuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, ErrUnauthorized},
		{"rate limited", http.StatusTooManyRequests, ErrRateLimited},
		{"server error", http.StatusInternalServerError, ErrBadResponse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeOpenAIJSON(w, tc.status, `{"error":{"message":"boom","type":"api_error"}}`)
			}))
			t.Cleanup(srv.Close)
			cli := newOpenAIClientForTest(t, srv, nil)
			_, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("status=%d err=%v", tc.status, err)
			}
		})
	}
}

// TestOpenAIChatContextError 验证取消上下文保持标准库错误
func TestOpenAIChatContextError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	transport := &ctxBlockingTransport{started: make(chan struct{})}
	cli := newOpenAIClientForTest(t, srv, func(c *Config) {
		c.HTTPClient = &http.Client{Transport: transport}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := cli.Chat(ctx, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
		done <- err
	}()
	<-transport.started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

// TestOpenAIChatStream 验证流式增量、控制帧跳过与结束符
func TestOpenAIChatStream(t *testing.T) {
	t.Parallel()
	var got openAIRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writeOpenAISSE(w,
			`{"id":"c1","object":"chat.completion.chunk","created":0,"model":"gpt-4o","choices":[]}`,
			`{"id":"c1","object":"chat.completion.chunk","created":0,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":""},"finish_reason":null}]}`,
			`{"id":"c1","object":"chat.completion.chunk","created":0,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
			`[DONE]`,
		)
	}))
	t.Cleanup(srv.Close)
	cli := newOpenAIClientForTest(t, srv, nil)
	stream, err := cli.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if !got.Stream {
		t.Fatal("stream flag not set")
	}
	chunk, err := stream.Recv()
	if err != nil || chunk.Delta != "hi" || chunk.ID != "c1" || chunk.Model != "gpt-4o" {
		t.Fatalf("chunk=%+v err=%v", chunk, err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("after EOF: %v", err)
	}
}

// TestOpenAIChatStreamEmpty 验证流结束仍无文本时为协议异常
func TestOpenAIChatStreamEmpty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAISSE(w, `[DONE]`)
	}))
	t.Cleanup(srv.Close)
	cli := newOpenAIClientForTest(t, srv, nil)
	stream, err := cli.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if _, err := stream.Recv(); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err=%v", err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("after bad response: %v", err)
	}
}

// TestOpenAIChatStreamConnectError 验证建连失败直接返回映射错误
func TestOpenAIChatStreamConnectError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAIJSON(w, http.StatusTooManyRequests, `{"error":{"message":"slow down","type":"rate_limit_error"}}`)
	}))
	t.Cleanup(srv.Close)
	cli := newOpenAIClientForTest(t, srv, nil)
	stream, err := cli.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrRateLimited) || stream != nil {
		t.Fatalf("stream=%v err=%v", stream, err)
	}
}

// TestOpenAIChatStreamMalformedFrame 验证流内坏帧映射为协议异常
func TestOpenAIChatStreamMalformedFrame(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAISSE(w, `{not-json`)
	}))
	t.Cleanup(srv.Close)
	cli := newOpenAIClientForTest(t, srv, nil)
	stream, err := cli.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if _, err := stream.Recv(); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err=%v", err)
	}
}

// TestOpenAIMapError 覆盖 nil、上下文错误、无状态码错误与 API 错误状态
func TestOpenAIMapError(t *testing.T) {
	t.Parallel()
	if err := mapOpenAIError(nil); err != nil {
		t.Fatalf("nil=%v", err)
	}
	if err := mapOpenAIError(context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx=%v", err)
	}
	if err := mapOpenAIError(io.EOF); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("plain=%v", err)
	}
	apiErr := &openai.Error{StatusCode: http.StatusUnauthorized}
	if err := mapOpenAIError(apiErr); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("api err=%v", err)
	}
}

// fakeOpenAIDecoder 测试用 SSE 解码器，只记录关闭次数
type fakeOpenAIDecoder struct {
	closes   atomic.Int32
	closeErr error
}

// Event 返回空事件
func (d *fakeOpenAIDecoder) Event() ssestream.Event { return ssestream.Event{} }

// Next 立即结束
func (d *fakeOpenAIDecoder) Next() bool { return false }

// Close 记录关闭次数并返回预置错误
func (d *fakeOpenAIDecoder) Close() error {
	d.closes.Add(1)
	return d.closeErr
}

// Err 无读取错误
func (d *fakeOpenAIDecoder) Err() error { return nil }

// TestOpenAIInvalidRequestFailsBeforeNetwork 验证非法请求在两个入口都在发请求前失败
func TestOpenAIInvalidRequestFailsBeforeNetwork(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeOpenAIJSON(w, http.StatusOK, openAIChatBody)
	}))
	t.Cleanup(srv.Close)
	cli := newOpenAIClientForTest(t, srv, nil)
	if _, err := cli.Chat(context.Background(), ChatRequest{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Chat err=%v", err)
	}
	if _, err := cli.ChatStream(context.Background(), ChatRequest{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("ChatStream err=%v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("network calls=%d", got)
	}
}

// TestOpenAIReasoningRetryAlternateField 验证 reasoning 模型被拒后切回 legacy 字段
func TestOpenAIReasoningRetryAlternateField(t *testing.T) {
	t.Parallel()
	var (
		mu     sync.Mutex
		bodies []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		mu.Unlock()
		if strings.Contains(string(raw), "max_completion_tokens") {
			writeOpenAIJSON(w, http.StatusBadRequest, `{"error":{"message":"Unsupported parameter: 'max_completion_tokens'","type":"invalid_request_error"}}`)
			return
		}
		writeOpenAIJSON(w, http.StatusOK, openAIChatBody)
	}))
	t.Cleanup(srv.Close)
	cli := newOpenAIClientForTest(t, srv, func(c *Config) {
		c.Model = "o1-mini"
	})
	resp, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	mu.Lock()
	gotBodies := append([]string(nil), bodies...)
	mu.Unlock()
	if resp.Message.Content != "hi" || len(gotBodies) != 2 {
		t.Fatalf("resp=%+v calls=%d", resp, len(gotBodies))
	}
	if !strings.Contains(gotBodies[1], `"max_tokens"`) || strings.Contains(gotBodies[1], `"max_completion_tokens"`) {
		t.Fatalf("second body=%s", gotBodies[1])
	}
}

// TestOpenAIDoesNotRetryUnrelatedBadRequest 验证与 max token 字段无关的 400 不重试
func TestOpenAIDoesNotRetryUnrelatedBadRequest(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeOpenAIJSON(w, http.StatusBadRequest, `{"error":{"message":"invalid request","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(srv.Close)
	cli := newOpenAIClientForTest(t, srv, func(c *Config) {
		c.Model = "o1-mini"
	})
	if _, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err=%v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls=%d", got)
	}
}

// TestOpenAIPumpClose 验证关闭只执行一次并透传底层错误
func TestOpenAIPumpClose(t *testing.T) {
	t.Parallel()
	dec := &fakeOpenAIDecoder{closeErr: io.ErrClosedPipe}
	p := &openaiPump{
		stream: ssestream.NewStream[openai.ChatCompletionChunk](dec, nil),
		mapErr: mapOpenAIError,
	}
	if err := p.close(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("first close: %v", err)
	}
	if err := p.close(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("repeat close: %v", err)
	}
	if got := dec.closes.Load(); got != 1 {
		t.Fatalf("close calls=%d", got)
	}
}
