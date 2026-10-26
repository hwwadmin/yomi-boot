// Anthropic 协议适配单测。
//
// @author zwei

package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// anthropicRequest 是测试侧解析的请求体
type anthropicRequest struct {
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	Stream      bool    `json:"stream"`
	System      []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"system"`
	Messages []struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"messages"`
}

// newAnthropicClientForTest 构造指向测试服务器的 Anthropic 客户端
func newAnthropicClientForTest(t *testing.T, srv *httptest.Server, mutate func(*Config)) *anthropicClient {
	t.Helper()
	cfg := Config{
		Protocol: ProtocolAnthropic,
		APIURL:   srv.URL,
		APIKey:   "sk-ant-test",
		Model:    "claude-test",
	}
	if mutate != nil {
		mutate(&cfg)
	}
	cli, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl, ok := cli.(*anthropicClient)
	if !ok {
		t.Fatalf("type=%T", cli)
	}
	return impl
}

// writeAnthropicJSON 写 JSON 响应
func writeAnthropicJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// writeAnthropicSSE 写 SSE 响应
func writeAnthropicSSE(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	for _, e := range events {
		_, _ = io.WriteString(w, e)
	}
}

// anthropicChatBody 是同步 Chat 的成功响应体
const anthropicChatBody = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
"content":[{"type":"text","text":"hello "},{"type":"text","text":"world"}],
"stop_reason":"end_turn","stop_sequence":null,
"usage":{"input_tokens":3,"output_tokens":5}}`

// TestAnthropicChatMapsRequestAndResponse 验证同步对话的请求映射、头部与响应结构
func TestAnthropicChatMapsRequestAndResponse(t *testing.T) {
	t.Parallel()
	var got anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			t.Errorf("method=%s path=%s", r.Method, r.URL.Path)
		}
		if gotKey := r.Header.Get("x-api-key"); gotKey != "sk-ant-test" {
			t.Errorf("x-api-key=%q", gotKey)
		}
		if v := r.Header.Get("anthropic-version"); v != "2024-01-01" {
			t.Errorf("anthropic-version=%q", v)
		}
		if v := r.Header.Get("x-gw"); v != "gw-1" {
			t.Errorf("x-gw=%q", v)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeAnthropicJSON(w, http.StatusOK, anthropicChatBody)
	}))
	t.Cleanup(srv.Close)

	temp := 0.3
	maxTokens := 1024
	cli := newAnthropicClientForTest(t, srv, func(c *Config) {
		c.AnthropicVersion = "2024-01-01"
		c.ExtraHeaders = map[string]string{"x-gw": "gw-1"}
		c.HTTPClient = srv.Client()
		c.Timeout = 5 * time.Second
	})
	resp, err := cli.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: "s1"},
			{Role: RoleUser, Content: "u1"},
			{Role: RoleSystem, Content: "s2"},
			{Role: RoleAssistant, Content: "a1"},
			{Role: RoleUser, Content: "u2"},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.ID != "msg_1" || resp.Model != "claude-test" || resp.Message.Role != RoleAssistant {
		t.Fatalf("resp=%+v", resp)
	}
	if resp.Message.Content != "hello world" {
		t.Fatalf("content=%q", resp.Message.Content)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 3 || resp.Usage.CompletionTokens != 5 || resp.Usage.TotalTokens != 8 {
		t.Fatalf("usage=%+v", resp.Usage)
	}

	if got.Model != "claude-test" || got.MaxTokens != 1024 || got.Temperature != 0.3 || got.Stream {
		t.Fatalf("body=%+v", got)
	}
	if len(got.System) != 1 || got.System[0].Text != "s1\ns2" {
		t.Fatalf("system=%+v", got.System)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("messages=%+v", got.Messages)
	}
	wantRoles := []string{"user", "assistant", "user"}
	for i, role := range wantRoles {
		if got.Messages[i].Role != role {
			t.Fatalf("messages[%d].role=%q", i, got.Messages[i].Role)
		}
	}
	if len(got.Messages[0].Content) != 1 || got.Messages[0].Content[0].Type != "text" || got.Messages[0].Content[0].Text != "u1" {
		t.Fatalf("messages[0].content=%+v", got.Messages[0].Content)
	}
}

// TestAnthropicChatErrorStatuses 验证 HTTP 状态到哨兵错误的映射
func TestAnthropicChatErrorStatuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"bad request", http.StatusBadRequest, ErrBadResponse},
		{"unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, ErrUnauthorized},
		{"rate limited", http.StatusTooManyRequests, ErrRateLimited},
		{"server error", http.StatusInternalServerError, ErrBadResponse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeAnthropicJSON(w, tc.status, `{"type":"error","error":{"type":"api_error","message":"boom"}}`)
			}))
			t.Cleanup(srv.Close)
			cli := newAnthropicClientForTest(t, srv, nil)
			_, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("status=%d err=%v", tc.status, err)
			}
		})
	}
}

// TestAnthropicChatBadResponses 验证空响应与无文本响应均为协议异常
func TestAnthropicChatBadResponses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{"null response", `null`},
		{"no content", `{"id":"m","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":"end_turn","usage":{}}`},
		{"only non-text", `{"id":"m","type":"message","role":"assistant","model":"claude-test","content":[{"type":"thinking","thinking":"hmm","signature":"sig"}],"stop_reason":"end_turn","usage":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeAnthropicJSON(w, http.StatusOK, tc.body)
			}))
			t.Cleanup(srv.Close)
			cli := newAnthropicClientForTest(t, srv, nil)
			resp, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
			if !errors.Is(err, ErrBadResponse) || resp != nil {
				t.Fatalf("resp=%+v err=%v", resp, err)
			}
		})
	}
}

// ctxBlockingTransport 阻塞到请求上下文结束，模拟悬挂请求
type ctxBlockingTransport struct {
	started chan struct{}
}

// RoundTrip 等待请求上下文结束并返回其错误
func (t *ctxBlockingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	close(t.started)
	<-req.Context().Done()
	return nil, req.Context().Err()
}

// TestAnthropicChatContextError 验证取消上下文保持标准库错误
func TestAnthropicChatContextError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	transport := &ctxBlockingTransport{started: make(chan struct{})}
	cli := newAnthropicClientForTest(t, srv, func(c *Config) {
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

// TestAnthropicChatDefaults 验证未设置采样参数时回退配置缺省
func TestAnthropicChatDefaults(t *testing.T) {
	t.Parallel()
	var got anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writeAnthropicJSON(w, http.StatusOK, `{"id":"m","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":0,"output_tokens":0}}`)
	}))
	t.Cleanup(srv.Close)
	temp := 0.7
	maxTokens := 2048
	cli := newAnthropicClientForTest(t, srv, func(c *Config) {
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
	if got.Temperature != 0.7 || got.MaxTokens != 2048 {
		t.Fatalf("body=%+v", got)
	}
}

// anthropicStreamEvents 是流式响应的固定事件序列
func anthropicStreamEvents() []string {
	return []string{
		"event: message_start\n" +
			`data: {"type":"message_start","message":{"id":"msg_s","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}` + "\n\n",
		"event: content_block_start\n" +
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n",
		"event: content_block_delta\n" +
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}` + "\n\n",
		"event: content_block_delta\n" +
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hel"}}` + "\n\n",
		"event: content_block_delta\n" +
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}` + "\n\n",
		"event: content_block_stop\n" +
			`data: {"type":"content_block_stop","index":0}` + "\n\n",
		"event: message_stop\n" +
			`data: {"type":"message_stop"}` + "\n\n",
	}
}

// TestAnthropicChatStream 验证流式增量、控制帧跳过与结束符
func TestAnthropicChatStream(t *testing.T) {
	t.Parallel()
	var got anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writeAnthropicSSE(w, anthropicStreamEvents()...)
	}))
	t.Cleanup(srv.Close)
	cli := newAnthropicClientForTest(t, srv, nil)
	stream, err := cli.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if !got.Stream {
		t.Fatal("stream flag not set")
	}
	first, err := stream.Recv()
	if err != nil || first.Delta != "hel" || first.ID != "msg_s" || first.Model != "claude-test" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := stream.Recv()
	if err != nil || second.Delta != "lo" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	// 结束后重复读取仍为未包装 EOF
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("after EOF: %v", err)
	}
}

// TestAnthropicChatStreamEmpty 验证流结束仍无文本时为协议异常
func TestAnthropicChatStreamEmpty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAnthropicSSE(w, "event: message_stop\n"+`data: {"type":"message_stop"}`+"\n\n")
	}))
	t.Cleanup(srv.Close)
	cli := newAnthropicClientForTest(t, srv, nil)
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

// TestAnthropicChatStreamConnectError 验证建连失败直接返回映射错误
func TestAnthropicChatStreamConnectError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAnthropicJSON(w, http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	}))
	t.Cleanup(srv.Close)
	cli := newAnthropicClientForTest(t, srv, nil)
	stream, err := cli.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrRateLimited) || stream != nil {
		t.Fatalf("stream=%v err=%v", stream, err)
	}
}

// TestAnthropicChatStreamErrorEvent 验证流内 error 事件映射为协议异常
func TestAnthropicChatStreamErrorEvent(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAnthropicSSE(w,
			"event: message_start\n"+
				`data: {"type":"message_start","message":{"id":"msg_s","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`+"\n\n",
			"event: error\n"+
				`data: {"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`+"\n\n",
		)
	}))
	t.Cleanup(srv.Close)
	cli := newAnthropicClientForTest(t, srv, nil)
	stream, err := cli.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if _, err := stream.Recv(); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err=%v", err)
	}
}

// TestAnthropicChatStreamCanceled 验证建连前取消保持上下文错误
func TestAnthropicChatStreamCanceled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAnthropicSSE(w, anthropicStreamEvents()...)
	}))
	t.Cleanup(srv.Close)
	cli := newAnthropicClientForTest(t, srv, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cli.ChatStream(ctx, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

// TestAnthropicBuildParams 验证系统提示拼接、轮次校验与采样参数覆盖
func TestAnthropicBuildParams(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	temp := 0.2
	maxTokens := 128
	cli := newAnthropicClientForTest(t, srv, func(c *Config) {
		c.Temperature = &temp
		c.MaxTokens = &maxTokens
	})

	t.Run("defaults", func(t *testing.T) {
		params, err := cli.buildParams(ChatRequest{Messages: []Message{{Role: RoleUser, Content: "u"}}})
		if err != nil {
			t.Fatalf("buildParams: %v", err)
		}
		if !params.Temperature.Valid() || params.Temperature.Value != 0.2 {
			t.Fatalf("temperature=%+v", params.Temperature)
		}
		if params.MaxTokens != 128 {
			t.Fatalf("max tokens=%d", params.MaxTokens)
		}
		if len(params.System) != 0 {
			t.Fatalf("system=%+v", params.System)
		}
	})

	t.Run("overrides", func(t *testing.T) {
		reqTemp := 0.9
		reqMax := 16
		params, err := cli.buildParams(ChatRequest{
			Messages:    []Message{{Role: RoleSystem, Content: "s1"}, {Role: RoleUser, Content: "u"}, {Role: RoleSystem, Content: "s2"}},
			Temperature: &reqTemp,
			MaxTokens:   &reqMax,
		})
		if err != nil {
			t.Fatalf("buildParams: %v", err)
		}
		if !params.Temperature.Valid() || params.Temperature.Value != 0.9 || params.MaxTokens != 16 {
			t.Fatalf("params=%+v", params)
		}
		if len(params.System) != 1 || params.System[0].Text != "s1\ns2" {
			t.Fatalf("system=%+v", params.System)
		}
		if len(params.Messages) != 1 || params.Messages[0].Content[0].OfText.Text != "u" {
			t.Fatalf("messages=%+v", params.Messages)
		}
	})

	invalid := []struct {
		name string
		msgs []Message
	}{
		{"empty", nil},
		{"system only", []Message{{Role: RoleSystem, Content: "s"}}},
		{"assistant first", []Message{{Role: RoleAssistant, Content: "a"}, {Role: RoleUser, Content: "u"}}},
		{"consecutive user", []Message{{Role: RoleUser, Content: "u1"}, {Role: RoleUser, Content: "u2"}}},
		{"consecutive assistant", []Message{{Role: RoleUser, Content: "u"}, {Role: RoleAssistant, Content: "a1"}, {Role: RoleAssistant, Content: "a2"}}},
		{"unknown role", []Message{{Role: Role("tool"), Content: "x"}}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cli.buildParams(ChatRequest{Messages: tc.msgs}); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestValidateAnthropicTurns 覆盖轮次校验的非用户中间角色分支
func TestValidateAnthropicTurns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		msgs []Message
		ok   bool
	}{
		{"user only", []Message{{Role: RoleUser, Content: "u"}}, true},
		{"alternating", []Message{{Role: RoleUser, Content: "u"}, {Role: RoleAssistant, Content: "a"}, {Role: RoleUser, Content: "u"}}, true},
		{"system ignored", []Message{{Role: RoleSystem, Content: "s"}, {Role: RoleUser, Content: "u"}}, true},
		{"empty", nil, false},
		{"starts assistant", []Message{{Role: RoleAssistant, Content: "a"}}, false},
		{"same role", []Message{{Role: RoleUser, Content: "u"}, {Role: RoleUser, Content: "u"}}, false},
		{"unknown second role", []Message{{Role: RoleUser, Content: "u"}, {Role: Role("tool"), Content: "t"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAnthropicTurns(tc.msgs)
			if tc.ok && err != nil {
				t.Fatalf("err=%v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestJoinSystemPrompts 验证系统提示按出现顺序换行拼接
func TestJoinSystemPrompts(t *testing.T) {
	t.Parallel()
	if got := joinSystemPrompts(nil); got != "" {
		t.Fatalf("got %q", got)
	}
	got := joinSystemPrompts([]Message{
		{Role: RoleUser, Content: "u"},
		{Role: RoleSystem, Content: "a"},
		{Role: RoleSystem, Content: "b"},
	})
	if got != "a\nb" {
		t.Fatalf("got %q", got)
	}
}

// TestCollectAnthropicText 验证只拼接文本块
func TestCollectAnthropicText(t *testing.T) {
	t.Parallel()
	blocks := []anthropic.ContentBlockUnion{
		{Type: "thinking", Thinking: "hmm"},
		{Type: "text", Text: "a"},
		{Type: "text", Text: "b"},
	}
	if got := collectAnthropicText(blocks); got != "ab" {
		t.Fatalf("got %q", got)
	}
	if got := collectAnthropicText(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

// TestAnthropicUsage 验证用量为零时返回 nil
func TestAnthropicUsage(t *testing.T) {
	t.Parallel()
	if got := anthropicUsage(0, 0); got != nil {
		t.Fatalf("got %+v", got)
	}
	if got := anthropicUsage(-1, 0); got != nil {
		t.Fatalf("got %+v", got)
	}
	got := anthropicUsage(2, 3)
	if got == nil || got.PromptTokens != 2 || got.CompletionTokens != 3 || got.TotalTokens != 5 {
		t.Fatalf("got %+v", got)
	}
}

// TestAnthropicMapErr 覆盖 nil、上下文错误与普通错误三类
func TestAnthropicMapErr(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	cli := newAnthropicClientForTest(t, srv, nil)
	if err := cli.mapErr(nil); err != nil {
		t.Fatalf("nil err=%v", err)
	}
	if err := cli.mapErr(context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ctx err=%v", err)
	}
	if err := cli.mapErr(io.EOF); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("plain err=%v", err)
	}
}

// fakeAnthropicDecoder 测试用 SSE 解码器，只记录关闭次数
type fakeAnthropicDecoder struct {
	closes   atomic.Int32
	closeErr error
}

// Event 返回空事件
func (d *fakeAnthropicDecoder) Event() ssestream.Event { return ssestream.Event{} }

// Next 立即结束
func (d *fakeAnthropicDecoder) Next() bool { return false }

// Close 记录关闭次数并返回预置错误
func (d *fakeAnthropicDecoder) Close() error {
	d.closes.Add(1)
	return d.closeErr
}

// Err 无读取错误
func (d *fakeAnthropicDecoder) Err() error { return nil }

// TestAnthropicPumpClose 验证关闭只执行一次并透传底层错误
func TestAnthropicPumpClose(t *testing.T) {
	t.Parallel()
	dec := &fakeAnthropicDecoder{closeErr: io.ErrClosedPipe}
	p := &anthropicPump{
		stream: ssestream.NewStream[anthropic.MessageStreamEventUnion](dec, nil),
		mapErr: nil,
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

// TestNewAnthropicClientOptions 验证自定义版本头、扩展头与 HTTP 客户端注入
func TestNewAnthropicClientOptions(t *testing.T) {
	t.Parallel()
	var version, header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version = r.Header.Get("anthropic-version")
		header = r.Header.Get("x-extra")
		writeAnthropicJSON(w, http.StatusOK, `{"id":"m","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{}}`)
	}))
	t.Cleanup(srv.Close)
	cli := newAnthropicClientForTest(t, srv, func(c *Config) {
		c.AnthropicVersion = " 2025-01-01 "
		c.ExtraHeaders = map[string]string{" X-Extra ": "v"}
		c.HTTPClient = srv.Client()
	})
	if _, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if version != "2025-01-01" || header != "v" {
		t.Fatalf("version=%q header=%q", version, header)
	}
}

// TestAnthropicInvalidRequestFailsBeforeNetwork 验证非法请求在两个入口都在发请求前失败
func TestAnthropicInvalidRequestFailsBeforeNetwork(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeAnthropicJSON(w, http.StatusOK, anthropicChatBody)
	}))
	t.Cleanup(srv.Close)
	cli := newAnthropicClientForTest(t, srv, nil)
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

// TestAnthropicBuildParamsRejectsBlankContent 验证非 system 空白内容在发请求前被拒绝
func TestAnthropicBuildParamsRejectsBlankContent(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	cli := newAnthropicClientForTest(t, srv, nil)
	cases := [][]Message{
		{{Role: RoleUser, Content: ""}},
		{{Role: RoleUser, Content: " \t"}},
		{{Role: RoleSystem, Content: "s"}, {Role: RoleAssistant, Content: "\n"}},
	}
	for _, msgs := range cases {
		if _, err := cli.buildParams(ChatRequest{Messages: msgs}); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("msgs=%+v err=%v", msgs, err)
		}
	}
}
