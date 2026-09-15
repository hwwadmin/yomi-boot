// Package aiclient 单元测试
//
// @author zwei
package aiclient

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

	"gitee.com/hq_project/yomi-boot/errors"
)

// userReq 构造仅含一条用户消息的请求
func userReq(content string) ChatRequest {
	return ChatRequest{Messages: []Message{{Role: RoleUser, Content: content}}}
}

// mustClient 构造客户端，失败则结束测试
func mustClient(t *testing.T, cfg Config) AIClient {
	t.Helper()
	cli, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return cli
}

// validCfg 构造合法测试配置
func validCfg(protocol Protocol, apiURL string, hc *http.Client) Config {
	return Config{
		Protocol:     protocol,
		APIURL:       apiURL,
		APIKey:       "test-key",
		DefaultModel: "test-model",
		Timeout:      5 * time.Second,
		HTTPClient:   hc,
	}
}

// TestTimeoutZeroDefaultsTo60s 验证 Timeout=0 回退 60s
func TestTimeoutZeroDefaultsTo60s(t *testing.T) {
	t.Parallel()
	if got := (Config{}).timeoutOrDefault(); got != 60*time.Second {
		t.Fatalf("got %v", got)
	}
}

// TestNewRejectsInvalidConfig 验证协议、修剪后的地址与密钥、负超时、保留头
func TestNewRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "unknown protocol", cfg: Config{Protocol: "foo", APIURL: "http://x", APIKey: "k"}},
		{name: "empty url", cfg: Config{Protocol: ProtocolOpenAI, APIURL: "  ", APIKey: "k"}},
		{name: "empty key", cfg: Config{Protocol: ProtocolOpenAI, APIURL: "http://x", APIKey: " \t"}},
		{name: "negative timeout", cfg: Config{Protocol: ProtocolOpenAI, APIURL: "http://x", APIKey: "k", Timeout: -time.Second}},
		{name: "reserved authorization", cfg: Config{Protocol: ProtocolOpenAI, APIURL: "http://x", APIKey: "k", ExtraHeaders: map[string]string{"Authorization": "x"}}},
		{name: "reserved x-api-key", cfg: Config{Protocol: ProtocolAnthropic, APIURL: "http://x", APIKey: "k", ExtraHeaders: map[string]string{"X-Api-Key": "x"}}},
		{name: "reserved anthropic-version", cfg: Config{Protocol: ProtocolAnthropic, APIURL: "http://x", APIKey: "k", ExtraHeaders: map[string]string{"anthropic-version": "2023-06-01"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tc.cfg)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

// TestNewTrimsAndCopiesExtraHeaders 验证修剪与扩展头拷贝隔离
func TestNewTrimsAndCopiesExtraHeaders(t *testing.T) {
	t.Parallel()

	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Tenant")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openaiChatBody("ok")))
	}))
	t.Cleanup(srv.Close)

	headers := map[string]string{"X-Tenant": "alpha"}
	cfg := validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client())
	cfg.ExtraHeaders = headers
	cfg.APIURL = "  " + srv.URL + "/v1  "
	cfg.APIKey = "  test-key  "
	cli := mustClient(t, cfg)
	headers["X-Tenant"] = "mutated"

	_, err := cli.Chat(context.Background(), userReq("hi"))
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if gotHeader != "alpha" {
		t.Fatalf("ExtraHeaders 未拷贝: got %q", gotHeader)
	}
}

// TestMustNewPanicsOnInvalidConfig 验证 MustNew 失败立即崩溃且不回显完整密钥
func TestMustNewPanicsOnInvalidConfig(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(error)
		if !ok || !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("panic value %v", r)
		}
		if strings.Contains(err.Error(), "super-secret-key") {
			t.Fatalf("error leaked api key: %v", err)
		}
	}()
	_ = MustNew(Config{Protocol: ProtocolOpenAI, APIURL: "http://x", APIKey: ""})
}

// TestValidateChatRequest 验证消息列表、角色、最大令牌数、模型回退
func TestValidateChatRequest(t *testing.T) {
	t.Parallel()

	if _, err := validateChatRequest(ChatRequest{}, "m"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty messages: %v", err)
	}
	zero := 0
	if _, err := validateChatRequest(ChatRequest{Messages: []Message{{Role: RoleUser}}, MaxTokens: &zero}, "m"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("maxTokens=0: %v", err)
	}
	if _, err := validateChatRequest(ChatRequest{Messages: []Message{{Role: Role("tool"), Content: "x"}}}, "m"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("unknown role: %v", err)
	}
	if _, err := validateChatRequest(userReq("x"), ""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty model: %v", err)
	}
	got, err := validateChatRequest(ChatRequest{Model: "req-model", Messages: []Message{{Role: RoleUser, Content: ""}}}, "")
	if err != nil || got != "req-model" {
		t.Fatalf("model from request: got=%q err=%v", got, err)
	}
	got, err = validateChatRequest(userReq("x"), "def")
	if err != nil || got != "def" {
		t.Fatalf("default model: got=%q err=%v", got, err)
	}
}

// TestValidateAnthropicTurns 验证抽掉系统提示后的轮次非法立即失败
func TestValidateAnthropicTurns(t *testing.T) {
	t.Parallel()

	if err := validateAnthropicTurns([]Message{{Role: RoleSystem, Content: "s"}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("only system: %v", err)
	}
	if err := validateAnthropicTurns([]Message{{Role: RoleAssistant, Content: "a"}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("assistant first: %v", err)
	}
	if err := validateAnthropicTurns([]Message{
		{Role: RoleUser, Content: "u1"},
		{Role: RoleUser, Content: "u2"},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("consecutive user: %v", err)
	}
	if err := validateAnthropicTurns([]Message{
		{Role: RoleSystem, Content: "s"},
		{Role: RoleUser, Content: "u"},
		{Role: RoleAssistant, Content: "a"},
		{Role: RoleUser, Content: "u2"},
	}); err != nil {
		t.Fatalf("valid turns: %v", err)
	}
}

// TestJoinSystemPrompts 验证系统提示按出现顺序换行拼接
func TestJoinSystemPrompts(t *testing.T) {
	t.Parallel()
	got := joinSystemPrompts([]Message{
		{Role: RoleSystem, Content: "a"},
		{Role: RoleUser, Content: "u"},
		{Role: RoleSystem, Content: "b"},
	})
	if got != "a\nb" {
		t.Fatalf("got %q", got)
	}
}

// TestMapStatusErrorSentinels 验证能取出状态码时按错误链命中哨兵
func TestMapStatusErrorSentinels(t *testing.T) {
	t.Parallel()
	cause := errors.New("upstream")
	if err := mapStatusError(http.StatusUnauthorized, cause, false); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401: %v", err)
	}
	if err := mapStatusError(http.StatusForbidden, cause, false); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("403: %v", err)
	}
	if err := mapStatusError(http.StatusTooManyRequests, cause, false); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("429: %v", err)
	}
	if err := mapStatusError(http.StatusBadRequest, cause, false); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("400: %v", err)
	}
	if err := mapStatusError(http.StatusInternalServerError, cause, false); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("500: %v", err)
	}
	if err := mapStatusError(0, cause, true); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("empty assistant: %v", err)
	}
	err := mapStatusError(0, cause, false)
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrRateLimited) || errors.Is(err, ErrBadResponse) || errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("no status should be system wrap, got %v", err)
	}
}

// openaiChatBody 构造同步成功响应体
func openaiChatBody(text string) string {
	return `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"` + text + `"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
}

// openaiEmptyChoicesBody 构造空选项列表的同步响应体
func openaiEmptyChoicesBody() string {
	return `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"test-model","choices":[]}`
}

// writeSSE 写出流式事件并刷新
func writeSSE(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	for _, event := range events {
		_, _ = io.WriteString(w, strings.TrimRight(event, "\n")+"\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// openaiChunkData 构造流式增量帧
func openaiChunkData(delta string) string {
	return `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"content":"` + delta + `"},"finish_reason":null}]}`
}

// anthropicMessageBody 构造同步成功响应体
func anthropicMessageBody(text string) string {
	return `{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"` + text + `"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":2,"output_tokens":3}}`
}

// newStatusServer 启动固定路径与状态码的测试服务
func newStatusServer(t *testing.T, path string, status int, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestOpenAIChatMapsResponse 验证 OpenAI 同步对话映射且角色为助手
func TestOpenAIChatMapsResponse(t *testing.T) {
	t.Parallel()
	srv := newStatusServer(t, "/v1/chat/completions", http.StatusOK, openaiChatBody("hello"))
	cli := mustClient(t, validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client()))
	resp, err := cli.Chat(context.Background(), userReq("hi"))
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Message.Role != RoleAssistant || resp.Message.Content != "hello" {
		t.Fatalf("resp=%+v", resp)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 2 {
		t.Fatalf("usage=%+v", resp.Usage)
	}
}

// TestOpenAIChatEmptyAssistant 验证空助手文本映射 ErrBadResponse
func TestOpenAIChatEmptyAssistant(t *testing.T) {
	t.Parallel()
	srv := newStatusServer(t, "/v1/chat/completions", http.StatusOK, openaiEmptyChoicesBody())
	cli := mustClient(t, validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client()))
	_, err := cli.Chat(context.Background(), userReq("hi"))
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("got %v", err)
	}
}

// TestOpenAIHTTPSentinels 验证 401/429/400 哨兵映射
func TestOpenAIHTTPSentinels(t *testing.T) {
	t.Parallel()
	body := `{"error":{"message":"x","type":"invalid_request_error"}}`
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusBadRequest, ErrInvalidArgument},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			t.Parallel()
			srv := newStatusServer(t, "/v1/chat/completions", tc.status, body)
			cli := mustClient(t, validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client()))
			_, err := cli.Chat(context.Background(), userReq("hi"))
			if !errors.Is(err, tc.want) {
				t.Fatalf("status %d: got %v", tc.status, err)
			}
		})
	}
}

// TestOpenAIChatTimeoutIsNotEOF 验证同步超时返回 DeadlineExceeded
func TestOpenAIChatTimeoutIsNotEOF(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openaiChatBody("late")))
	}))
	t.Cleanup(srv.Close)
	cfg := validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client())
	cfg.Timeout = 30 * time.Millisecond
	cli := mustClient(t, cfg)
	_, err := cli.Chat(context.Background(), userReq("hi"))
	if err == nil || errors.Is(err, io.EOF) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

// TestOpenAIParallelChat 验证同一实例并行 Chat 可过竞态检测
func TestOpenAIParallelChat(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openaiChatBody("ok")))
	}))
	t.Cleanup(srv.Close)
	cli := mustClient(t, validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client()))
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cli.Chat(context.Background(), userReq("hi"))
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("parallel Chat: %v", err)
		}
	}
}

// TestOpenAIChatStreamRecv 验证流式增量与未包装 EOF
func TestOpenAIChatStreamRecv(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w,
			openaiChunkData("he"),
			openaiChunkData("llo"),
			"data: [DONE]",
		)
	}))
	t.Cleanup(srv.Close)
	cli := mustClient(t, validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client()))
	stream, err := cli.ChatStream(context.Background(), userReq("hi"))
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer stream.Close()
	var got strings.Builder
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		got.WriteString(chunk.Delta)
	}
	if got.String() != "hello" {
		t.Fatalf("delta=%q", got.String())
	}
	_, err = stream.Recv()
	if err != io.EOF {
		t.Fatalf("after end Recv want raw EOF, got %v", err)
	}
}

// TestOpenAIChatStreamConnectFailure 验证建连失败不返回半开流
func TestOpenAIChatStreamConnectFailure(t *testing.T) {
	t.Parallel()
	srv := newStatusServer(t, "/v1/chat/completions", http.StatusUnauthorized, `{"error":{"message":"no"}}`)
	cli := mustClient(t, validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client()))
	stream, err := cli.ChatStream(context.Background(), userReq("hi"))
	if stream != nil {
		t.Fatal("expected nil stream")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
}

// TestOpenAIStreamTimeoutNotEOF 验证流超时当次为 DeadlineExceeded，其后 EOF
func TestOpenAIStreamTimeoutNotEOF(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, openaiChunkData("hi"))
		time.Sleep(400 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)
	cfg := validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client())
	cfg.Timeout = 150 * time.Millisecond
	cli := mustClient(t, cfg)
	stream, err := cli.ChatStream(context.Background(), userReq("hi"))
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer stream.Close()
	chunk, err := stream.Recv()
	if err != nil {
		t.Fatalf("first Recv: %v", err)
	}
	if chunk.Delta != "hi" {
		t.Fatalf("delta=%q", chunk.Delta)
	}
	_, err = stream.Recv()
	if errors.Is(err, io.EOF) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout Recv want DeadlineExceeded, got %v", err)
	}
	_, err = stream.Recv()
	if err != io.EOF {
		t.Fatalf("after timeout Recv want raw EOF, got %v", err)
	}
}

// TestAnthropicChatSystemAndTurns 验证系统提示拼入请求且轮次非法不发网
func TestAnthropicChatSystemAndTurns(t *testing.T) {
	t.Parallel()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicMessageBody("ok")))
	}))
	t.Cleanup(srv.Close)
	cli := mustClient(t, validCfg(ProtocolAnthropic, srv.URL, srv.Client()))

	_, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleSystem, Content: "s"}}})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("only system: %v", err)
	}

	resp, err := cli.Chat(context.Background(), ChatRequest{Messages: []Message{
		{Role: RoleSystem, Content: "sa"},
		{Role: RoleSystem, Content: "sb"},
		{Role: RoleUser, Content: "hi"},
	}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Message.Role != RoleAssistant || resp.Message.Content != "ok" {
		t.Fatalf("resp=%+v", resp)
	}
	sys, _ := body["system"].([]any)
	if len(sys) != 1 {
		t.Fatalf("system=%v", body["system"])
	}
	block, _ := sys[0].(map[string]any)
	if block["text"] != "sa\nsb" {
		t.Fatalf("system text=%v", block["text"])
	}
}

// TestAnthropicHTTPSentinels 验证 Anthropic 401/429 映射
func TestAnthropicHTTPSentinels(t *testing.T) {
	t.Parallel()
	srv := newStatusServer(t, "/v1/messages", http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"x"}}`)
	cli := mustClient(t, validCfg(ProtocolAnthropic, srv.URL, srv.Client()))
	_, err := cli.Chat(context.Background(), userReq("hi"))
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got %v", err)
	}
}

// TestAnthropicChatStreamRecv 验证 Anthropic 流式文本增量
func TestAnthropicChatStreamRecv(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w,
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}",
		)
	}))
	t.Cleanup(srv.Close)
	cli := mustClient(t, validCfg(ProtocolAnthropic, srv.URL, srv.Client()))
	stream, err := cli.ChatStream(context.Background(), userReq("hi"))
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer stream.Close()
	chunk, err := stream.Recv()
	if err != nil || chunk.Delta != "hi" {
		t.Fatalf("chunk=%v err=%v", chunk, err)
	}
	_, err = stream.Recv()
	if err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

// stubPump 测试用流读取器，可预置增量、错误与阻塞
type stubPump struct {
	mu     sync.Mutex    // mu 保护增量队列
	chunks []*ChatChunk  // chunks 是待返回的增量队列
	err    error         // err 是队列耗尽后的错误，空则结束
	block  chan struct{} // block 非空时读取先等待关闭
	closed atomic.Bool   // closed 记录是否已关闭
}

// next 弹出下一增量；可先阻塞；队列空时返回预置错误或结束
func (p *stubPump) next() (*ChatChunk, error) {
	if p.block != nil {
		<-p.block
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.chunks) == 0 {
		if p.err != nil {
			return nil, p.err
		}
		return nil, io.EOF
	}
	c := p.chunks[0]
	p.chunks = p.chunks[1:]
	return c, nil
}

// close 标记已关闭并解除读取阻塞；幂等
func (p *stubPump) close() error {
	p.closed.Store(true)
	if p.block != nil {
		select {
		case <-p.block:
		default:
			close(p.block)
		}
	}
	return nil
}

// TestStreamConcurrentRecvDoesNotClose 验证并发 Recv 返回非法参数且不关流
func TestStreamConcurrentRecvDoesNotClose(t *testing.T) {
	t.Parallel()
	p := &stubPump{block: make(chan struct{})}
	s := newChatStream(context.Background(), nil, p)
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := s.Recv()
		done <- err
	}()
	<-started
	time.Sleep(20 * time.Millisecond)
	_, err := s.Recv()
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("concurrent Recv: %v", err)
	}
	if p.closed.Load() {
		t.Fatal("concurrent Recv closed stream")
	}
	close(p.block)
	if err := <-done; err != io.EOF && !errors.Is(err, ErrBadResponse) {
		t.Fatalf("first Recv: %v", err)
	}
}

// TestStreamCloseThenRecvEOF 验证 Close 后再 Recv 为未包装 EOF
func TestStreamCloseThenRecvEOF(t *testing.T) {
	t.Parallel()
	p := &stubPump{chunks: []*ChatChunk{{Delta: "x"}}}
	s := newChatStream(context.Background(), nil, p)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close idempotent: %v", err)
	}
	_, err := s.Recv()
	if err != io.EOF {
		t.Fatalf("got %v", err)
	}
}

// TestStreamCloseUnblocksRecvAsEOF 验证 Close 使进行中 Recv 以 EOF 结束
func TestStreamCloseUnblocksRecvAsEOF(t *testing.T) {
	t.Parallel()
	p := &stubPump{block: make(chan struct{})}
	s := newChatStream(context.Background(), func() {}, p)
	done := make(chan error, 1)
	go func() {
		_, err := s.Recv()
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := <-done
	if err != io.EOF {
		t.Fatalf("in-flight Recv want EOF, got %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("caller Close rewritten as Canceled")
	}
}

// TestStreamCloseMapsPumpReadErrorToEOF 验证 Close 打断读时连接关闭类错误收口为未包装 EOF
func TestStreamCloseMapsPumpReadErrorToEOF(t *testing.T) {
	t.Parallel()
	p := &stubPump{block: make(chan struct{}), err: errors.New("use of closed network connection")}
	s := newChatStream(context.Background(), func() {}, p)
	done := make(chan error, 1)
	go func() {
		_, err := s.Recv()
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := <-done
	if err != io.EOF {
		t.Fatalf("in-flight Recv want raw EOF, got %v", err)
	}
}

// TestWrapCtxErrIdempotent 验证超时错误只包装一次
func TestWrapCtxErrIdempotent(t *testing.T) {
	t.Parallel()
	once := wrapCtxErr(context.DeadlineExceeded)
	twice := wrapCtxErr(once)
	if twice != once {
		t.Fatalf("second wrap changed identity: %v", twice)
	}
	if !errors.Is(twice, context.DeadlineExceeded) {
		t.Fatalf("lost DeadlineExceeded: %v", twice)
	}
	if strings.Count(twice.Error(), "aiclient: 请求已取消或超时") != 1 {
		t.Fatalf("wrapped more than once: %q", twice.Error())
	}
}

// TestStreamEmptyTextIsBadResponse 验证流结束无文本且非 Close 时为 ErrBadResponse
func TestStreamEmptyTextIsBadResponse(t *testing.T) {
	t.Parallel()
	s := newChatStream(context.Background(), nil, &stubPump{})
	_, err := s.Recv()
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("got %v", err)
	}
	_, err = s.Recv()
	if err != io.EOF {
		t.Fatalf("after bad response: %v", err)
	}
}

// TestStreamTimeoutThenEOF 验证当次超时非 EOF，之后为未包装 EOF
func TestStreamTimeoutThenEOF(t *testing.T) {
	t.Parallel()
	p := &stubPump{block: make(chan struct{}), err: context.DeadlineExceeded}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	s := newChatStream(ctx, cancel, p)
	time.Sleep(40 * time.Millisecond)
	_, err := s.Recv()
	if errors.Is(err, io.EOF) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	_, err = s.Recv()
	if err != io.EOF {
		t.Fatalf("after timeout: %v", err)
	}
}

// TestChatRequestRejectedBeforeNetwork 验证非法请求不发起 HTTP
func TestChatRequestRejectedBeforeNetwork(t *testing.T) {
	t.Parallel()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	t.Cleanup(srv.Close)
	cli := mustClient(t, validCfg(ProtocolOpenAI, srv.URL+"/v1", srv.Client()))
	_, err := cli.Chat(context.Background(), ChatRequest{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("got %v", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatal("unexpected HTTP")
	}
}
