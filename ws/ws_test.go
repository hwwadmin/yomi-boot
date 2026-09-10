// Package ws 的 Server/Client 门面联调测试
//
// @author zwei
package ws

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/ws/wsbase"
)

const testWait = 2 * time.Second

// testServerBiz 可观察的服务端 Handler
type testServerBiz struct {
	NopServerHandler
	mu sync.Mutex

	connIDs  []string
	messages map[string][][]byte

	connectCh chan string
	messageCh chan struct{}
}

// newTestServerBiz 创建可观察服务端 Handler
func newTestServerBiz() *testServerBiz {
	return &testServerBiz{
		messages:  make(map[string][][]byte),
		connectCh: make(chan string, 8),
		messageCh: make(chan struct{}, 64),
	}
}

// OnConnect 记录 ConnID
func (b *testServerBiz) OnConnect(ctx SessionContext) {
	id := ctx.ConnID()
	b.mu.Lock()
	b.connIDs = append(b.connIDs, id)
	b.mu.Unlock()
	select {
	case b.connectCh <- id:
	default:
	}
}

// OnMessage 按 ConnID 记录消息
func (b *testServerBiz) OnMessage(ctx SessionContext, msg Message) {
	defer msg.Close()
	cp := append([]byte(nil), msg.Bytes()...)
	id := ctx.ConnID()
	b.mu.Lock()
	b.messages[id] = append(b.messages[id], cp)
	b.mu.Unlock()
	select {
	case b.messageCh <- struct{}{}:
	default:
	}
}

// lastConnID 返回最近一次连接的 ConnID
func (b *testServerBiz) lastConnID(t *testing.T) string {
	t.Helper()
	select {
	case id := <-b.connectCh:
		if id == "" {
			t.Fatal("ConnID 为空")
		}
		return id
	case <-time.After(testWait):
		t.Fatal("等待服务端 OnConnect 超时")
		return ""
	}
}

// waitMessages 等待至少 n 条消息
func (b *testServerBiz) waitMessages(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-b.messageCh:
		case <-time.After(testWait):
			t.Fatalf("等待服务端消息超时 at %d/%d", i, n)
		}
	}
}

// testClientBiz 可观察的客户端 Handler
type testClientBiz struct {
	NopClientHandler
	mu sync.Mutex

	connected bool
	messages  [][]byte

	connectCh chan struct{}
	messageCh chan struct{}
}

// newTestClientBiz 创建可观察客户端 Handler
func newTestClientBiz() *testClientBiz {
	return &testClientBiz{
		connectCh: make(chan struct{}, 8),
		messageCh: make(chan struct{}, 64),
	}
}

// OnConnect 标记已连接
func (b *testClientBiz) OnConnect(PeerContext) {
	b.mu.Lock()
	b.connected = true
	b.mu.Unlock()
	select {
	case b.connectCh <- struct{}{}:
	default:
	}
}

// OnMessage 记录消息
func (b *testClientBiz) OnMessage(_ PeerContext, msg Message) {
	defer msg.Close()
	cp := append([]byte(nil), msg.Bytes()...)
	b.mu.Lock()
	b.messages = append(b.messages, cp)
	b.mu.Unlock()
	select {
	case b.messageCh <- struct{}{}:
	default:
	}
}

// waitConnect 等待客户端连通
func (b *testClientBiz) waitConnect(t *testing.T) {
	t.Helper()
	select {
	case <-b.connectCh:
	case <-time.After(testWait):
		t.Fatal("等待客户端 OnConnect 超时")
	}
}

// waitMessages 等待至少 n 条消息
func (b *testClientBiz) waitMessages(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-b.messageCh:
		case <-time.After(testWait):
			t.Fatalf("等待客户端消息超时 at %d/%d", i, n)
		}
	}
}

// startAppPair 用自定义 Handler 启动一对 Server/Client
func startAppPair(t *testing.T, srvBiz ServerHandler, cliBiz ClientHandler) (Server, Client, string) {
	t.Helper()
	t.Log("[流程] 启动 Server（ConnID 由服务端自动生成）")

	srv, err := NewServer(ServerConfig{
		ServerConfig: wsbase.ServerConfig{Addr: "127.0.0.1:0", Name: "app-srv"},
		Handler:      srvBiz,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		t.Log("[流程] Cleanup：关闭 Client 与 Server")
		cancel()
		_ = srv.Close(context.Background())
	})
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr, err := srv.ListenAddr()
	if err != nil {
		t.Fatalf("ListenAddr: %v", err)
	}
	t.Logf("[流程] Server 监听 %v", addr)

	cli, err := NewClient(ClientConfig{
		ClientConfig: wsbase.ClientConfig{Name: "app-cli"},
		Handler:      cliBiz,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	url := "ws://" + addr.String() + "/"
	t.Logf("[流程] Client Connect url=%s", url)
	if err := cli.Connect(context.Background(), url); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close(CloseNormalClosure, "test done") })

	var connID string
	if tb, ok := srvBiz.(*testServerBiz); ok {
		connID = tb.lastConnID(t)
		t.Logf("[流程] 服务端 OnConnect connID=%s", connID)
	}
	if tb, ok := cliBiz.(*testClientBiz); ok {
		tb.waitConnect(t)
		t.Log("[流程] 客户端 OnConnect")
	}
	return srv, cli, connID
}

// TestAppServerClientConnect 门面基本用例：连通后服务端拥有 ConnID
func TestAppServerClientConnect(t *testing.T) {
	t.Log("[用例] Server + Client 连接")
	srvBiz, cliBiz := newTestServerBiz(), newTestClientBiz()
	srv, cli, connID := startAppPair(t, srvBiz, cliBiz)

	if !cli.Connected() {
		t.Fatal("Client 应已连接")
	}
	if connID == "" || !srv.Has(connID) {
		t.Fatalf("服务端应登记自动生成的 ConnID, got %q", connID)
	}
	if srv.Len() != 1 {
		t.Fatalf("Len=%d", srv.Len())
	}
	t.Logf("[断言] Len=1 Has(%s)=true Client.Connected=true", connID)
}

// TestAppServerClientTextRoundTrip 文本往返：客户端无 ConnID，服务端用 ConnID 回写
func TestAppServerClientTextRoundTrip(t *testing.T) {
	t.Log("[用例] 文本往返")
	srvBiz, cliBiz := newTestServerBiz(), newTestClientBiz()
	srv, cli, connID := startAppPair(t, srvBiz, cliBiz)

	t.Log("[流程] Client.SendText hello")
	if err := cli.SendText(context.Background(), []byte("hello")); err != nil {
		t.Fatal(err)
	}
	srvBiz.waitMessages(t, 1)
	srvBiz.mu.Lock()
	got := srvBiz.messages[connID][0]
	srvBiz.mu.Unlock()
	if !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("服务端收到=%q", got)
	}
	t.Log("[断言] 服务端业务收到 hello")

	t.Logf("[流程] Server.SendText(connID=%s) 回写 world", connID)
	if err := srv.SendText(connID, []byte("world")); err != nil {
		t.Fatal(err)
	}
	cliBiz.waitMessages(t, 1)
	cliBiz.mu.Lock()
	got = cliBiz.messages[0]
	cliBiz.mu.Unlock()
	if !bytes.Equal(got, []byte("world")) {
		t.Fatalf("客户端收到=%q", got)
	}
	t.Log("[断言] 客户端业务收到 world（客户端仍不知 ConnID）")
}

// TestAppClientChunkedUpload 客户端分片上传
func TestAppClientChunkedUpload(t *testing.T) {
	const chunk = 4
	t.Logf("[用例] Client.SendReader 分片上传 chunk=%d", chunk)
	srvBiz, cliBiz := newTestServerBiz(), newTestClientBiz()
	_, cli, connID := startAppPair(t, srvBiz, cliBiz)

	payload := []byte("abcdefghijkl")
	t.Logf("[流程] SendReader 总长=%d", len(payload))
	if err := cli.SendReader(context.Background(), OpcodeBinary, bytes.NewReader(payload), WithChunkSize(chunk)); err != nil {
		t.Fatal(err)
	}
	srvBiz.waitMessages(t, 3)
	srvBiz.mu.Lock()
	parts := srvBiz.messages[connID]
	srvBiz.mu.Unlock()
	if joined := bytes.Join(parts, nil); !bytes.Equal(joined, payload) {
		t.Fatalf("拼接=%q want %q", joined, payload)
	}
	t.Log("[断言] 分片拼接正确")
}

// TestAppServerSendUnknownPeer 向不存在 ConnID 推送应失败
func TestAppServerSendUnknownPeer(t *testing.T) {
	t.Log("[用例] SendText 未知 ConnID")
	srv, err := NewServer(ServerConfig{
		ServerConfig: wsbase.ServerConfig{Addr: "127.0.0.1:0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer srv.Close(context.Background())
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	err = srv.SendText("nobody", []byte("x"))
	if err == nil {
		t.Fatal("应返回 ErrUnknownPeer")
	}
	t.Logf("[断言] 错误=%v", err)
}

// TestAppClientSendBeforeConnect 未连接发送应失败
func TestAppClientSendBeforeConnect(t *testing.T) {
	t.Log("[用例] 未 Connect 时 SendText")
	cli, err := NewClient(ClientConfig{})
	if err != nil {
		t.Fatal(err)
	}
	err = cli.SendText(context.Background(), []byte("x"))
	if err == nil {
		t.Fatal("应返回 ErrNotConnected")
	}
	t.Logf("[断言] 错误=%v", err)
}

// TestAppEscapeHatches 逃生舱可用
func TestAppEscapeHatches(t *testing.T) {
	t.Log("[用例] Hub/Underlying/Conn 逃生舱")
	srvBiz, cliBiz := newTestServerBiz(), newTestClientBiz()
	srv, cli, connID := startAppPair(t, srvBiz, cliBiz)

	if srv.Hub() == nil || srv.Underlying() == nil || srv.NativeUpgrader() == nil {
		t.Fatal("Server 逃生舱不应为空")
	}
	if _, ok := srv.GetConn(connID); !ok {
		t.Fatal("GetConn(connID) 应成功")
	}
	if cli.Underlying() == nil {
		t.Fatal("Client.Underlying 不应为空")
	}
	if _, ok := cli.Conn(); !ok {
		t.Fatal("Client.Conn 应成功")
	}
	t.Log("[断言] 逃生舱均可用")
}

// echoServerProcessor 服务端消息处理器：收到后原样回写
type echoServerProcessor struct {
	mu       sync.Mutex
	payloads []Payload
	connID   string
	ready    chan struct{}
	gotMsg   chan struct{}
}

// OnConnect 记录 ConnID
func (p *echoServerProcessor) OnConnect(ctx SessionContext) {
	p.mu.Lock()
	p.connID = ctx.ConnID()
	p.mu.Unlock()
	select {
	case p.ready <- struct{}{}:
	default:
	}
}

// OnDisconnect 空
func (p *echoServerProcessor) OnDisconnect(string, error) {}

// OnPayload 记录并回写
func (p *echoServerProcessor) OnPayload(ctx SessionContext, payload Payload) error {
	p.mu.Lock()
	p.payloads = append(p.payloads, payload)
	p.mu.Unlock()
	select {
	case p.gotMsg <- struct{}{}:
	default:
	}
	return ctx.SendBinary(payload.Data)
}

// echoClientProcessor 客户端消息处理器
type echoClientProcessor struct {
	mu       sync.Mutex
	payloads []Payload
	ready    chan struct{}
	gotMsg   chan struct{}
}

// OnConnect 标记就绪
func (p *echoClientProcessor) OnConnect(PeerContext) {
	select {
	case p.ready <- struct{}{}:
	default:
	}
}

// OnDisconnect 空
func (p *echoClientProcessor) OnDisconnect(error) {}

// OnPayload 记录载荷
func (p *echoClientProcessor) OnPayload(_ PeerContext, payload Payload) error {
	p.mu.Lock()
	p.payloads = append(p.payloads, payload)
	p.mu.Unlock()
	select {
	case p.gotMsg <- struct{}{}:
	default:
	}
	return nil
}

// TestMessageProcessorConfig 验证只配 Processor 的主路径
func TestMessageProcessorConfig(t *testing.T) {
	t.Log("[用例] Config.Processor 主路径")
	srvProc := &echoServerProcessor{
		ready:  make(chan struct{}, 1),
		gotMsg: make(chan struct{}, 8),
	}
	cliProc := &echoClientProcessor{
		ready:  make(chan struct{}, 1),
		gotMsg: make(chan struct{}, 8),
	}

	srv, err := NewServer(ServerConfig{
		ServerConfig: wsbase.ServerConfig{Addr: "127.0.0.1:0"},
		Processor:    srvProc,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer srv.Close(context.Background())
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	addr, err := srv.ListenAddr()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("[流程] Server 监听 %v", addr)

	cli, err := NewClient(ClientConfig{Processor: cliProc})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close(CloseNormalClosure, "done")
	if err := cli.Connect(context.Background(), "ws://"+addr.String()+"/"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-srvProc.ready:
		t.Log("[流程] 服务端 Processor OnConnect")
	case <-time.After(testWait):
		t.Fatal("等待服务端 Processor OnConnect 超时")
	}
	select {
	case <-cliProc.ready:
		t.Log("[流程] 客户端 Processor OnConnect")
	case <-time.After(testWait):
		t.Fatal("等待客户端 Processor OnConnect 超时")
	}

	want := []byte("proc-ping")
	if err := cli.SendBinary(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	select {
	case <-srvProc.gotMsg:
	case <-time.After(testWait):
		t.Fatal("等待服务端 OnPayload 超时")
	}
	select {
	case <-cliProc.gotMsg:
	case <-time.After(testWait):
		t.Fatal("等待客户端收到回写超时")
	}

	srvProc.mu.Lock()
	srvGot := append([]byte(nil), srvProc.payloads[0].Data...)
	srvProc.mu.Unlock()
	cliProc.mu.Lock()
	cliGot := append([]byte(nil), cliProc.payloads[0].Data...)
	cliProc.mu.Unlock()
	if !bytes.Equal(srvGot, want) || !bytes.Equal(cliGot, want) {
		t.Fatalf("srv=%q cli=%q want=%q", srvGot, cliGot, want)
	}
	t.Log("[断言] Processor 收发正确")
}

// countingServerHandler 嵌入默认 Handler，额外计数 OnMessage
type countingServerHandler struct {
	DefaultServerHandler
	mu    sync.Mutex
	count int
}

// OnMessage 计数后转交默认实现
func (h *countingServerHandler) OnMessage(ctx SessionContext, msg Message) {
	h.mu.Lock()
	h.count++
	h.mu.Unlock()
	h.DefaultServerHandler.OnMessage(ctx, msg)
}

// TestCustomHandlerStillUsesProcessor 自定义 Handler 嵌入默认实现并复用 Processor
func TestCustomHandlerStillUsesProcessor(t *testing.T) {
	t.Log("[用例] 自定义 Handler 嵌入 DefaultServerHandler")
	srvProc := &echoServerProcessor{
		ready:  make(chan struct{}, 1),
		gotMsg: make(chan struct{}, 8),
	}
	cliProc := &echoClientProcessor{
		ready:  make(chan struct{}, 1),
		gotMsg: make(chan struct{}, 8),
	}
	srvH := &countingServerHandler{
		DefaultServerHandler: DefaultServerHandler{Processor: srvProc},
	}

	srv, err := NewServer(ServerConfig{
		ServerConfig: wsbase.ServerConfig{Addr: "127.0.0.1:0"},
		Handler:      srvH,
		Processor:    &echoServerProcessor{}, // 应被静默忽略（不自动接入）
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer srv.Close(context.Background())
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	addr, err := srv.ListenAddr()
	if err != nil {
		t.Fatal(err)
	}

	cli, err := NewClient(ClientConfig{
		Handler: DefaultClientHandler{Processor: cliProc},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close(CloseNormalClosure, "done")
	if err := cli.Connect(context.Background(), "ws://"+addr.String()+"/"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-srvProc.ready:
	case <-time.After(testWait):
		t.Fatal("等待服务端 OnConnect 超时")
	}
	select {
	case <-cliProc.ready:
	case <-time.After(testWait):
		t.Fatal("等待客户端 OnConnect 超时")
	}

	want := []byte("embed-ping")
	if err := cli.SendBinary(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	select {
	case <-srvProc.gotMsg:
	case <-time.After(testWait):
		t.Fatal("等待嵌入 Handler 的 Processor 超时")
	}
	select {
	case <-cliProc.gotMsg:
	case <-time.After(testWait):
		t.Fatal("等待客户端回写超时")
	}

	srvH.mu.Lock()
	n := srvH.count
	srvH.mu.Unlock()
	if n != 1 {
		t.Fatalf("自定义 OnMessage 计数=%d want 1", n)
	}
	srvProc.mu.Lock()
	got := append([]byte(nil), srvProc.payloads[0].Data...)
	srvProc.mu.Unlock()
	if !bytes.Equal(got, want) {
		t.Fatalf("Processor 收到=%q", got)
	}
	t.Log("[断言] 自定义 Handler 加强后仍走嵌入的 Processor")
}
