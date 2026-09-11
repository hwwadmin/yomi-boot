// Package ws 联调测试：Processor 只消费消息；连通后主动发信在 Connect 之后发起
//
// @author zwei
package ws

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

const testWait = 2 * time.Second

// TestProcessorConnectAndText 连通后客户端发 hello；服务端 OnPayload 回 world；客户端 OnPayload 收妥
func TestProcessorConnectAndText(t *testing.T) {
	t.Log("[用例] 文本往返（Processor 只负责 OnPayload）")

	srvProc := newChatServerProcessor()
	cliProc := newChatClientProcessor()
	pair := startPairAsync(t, srvProc, cliProc)

	t.Log("[发送] Connect 成功后 Client.Send MsgText hello")
	if err := pair.cli.Send(MsgText, []byte("hello")); err != nil {
		t.Fatal(err)
	}

	cliProc.waitDone(t)
	t.Logf("[断言] 业务完成 connID=%s Connected=%v", srvProc.connID(), pair.cli.Connected())
	if !bytes.Equal(srvProc.received(), []byte("hello")) {
		t.Fatalf("服务端消费=%q want hello", srvProc.received())
	}
	if !bytes.Equal(cliProc.received(), []byte("world")) {
		t.Fatalf("客户端消费=%q want world", cliProc.received())
	}
	t.Log("[断言] 服务端 OnPayload 消费 hello 并回 world；客户端 OnPayload 消费 world")
}

// TestProcessorBinaryUpload 连通后客户端整包上传二进制；服务端 OnPayload 一次收齐并回执
func TestProcessorBinaryUpload(t *testing.T) {
	file := []byte("abcdefghijkl")
	t.Logf("[用例] 二进制整包上传 总长=%d", len(file))

	srvProc := newUploadServerProcessor(file)
	cliProc := newUploadClientProcessor()
	pair := startPairAsync(t, srvProc, cliProc)

	t.Log("[发送] Connect 成功后 Client.Send MsgBinary")
	if err := pair.cli.Send(MsgBinary, file); err != nil {
		t.Fatal(err)
	}

	cliProc.waitDone(t)
	if !bytes.Equal(srvProc.received(), file) {
		t.Fatalf("服务端收到=%q want %q", srvProc.received(), file)
	}
	if !bytes.Equal(cliProc.ack(), []byte("upload-ok")) {
		t.Fatalf("客户端回执=%q want upload-ok", cliProc.ack())
	}
	t.Log("[断言] 服务端一次 OnPayload 收齐并回执")
}

// TestProcessorBinaryDownload 连通后客户端请求；服务端 OnPayload 内整包下发
func TestProcessorBinaryDownload(t *testing.T) {
	file := []byte("hello-file!!")
	t.Logf("[用例] 请求下发二进制 总长=%d", len(file))

	srvProc := newDownloadServerProcessor(file)
	cliProc := newDownloadClientProcessor(file)
	pair := startPairAsync(t, srvProc, cliProc)

	t.Log("[发送] Connect 成功后 Client.Send MsgText GET_FILE")
	if err := pair.cli.Send(MsgText, []byte("GET_FILE")); err != nil {
		t.Fatal(err)
	}

	cliProc.waitDone(t)
	if !bytes.Equal(srvProc.request(), []byte("GET_FILE")) {
		t.Fatalf("服务端请求=%q want GET_FILE", srvProc.request())
	}
	if !bytes.Equal(cliProc.received(), file) {
		t.Fatalf("客户端收到=%q want %q", cliProc.received(), file)
	}
	t.Log("[断言] 服务端 OnPayload 下发；客户端一次收到完整载荷")
}

// ---------------------------------------------------------------------------
// 双协程启动：Server / Client 上下文隔离
// ---------------------------------------------------------------------------

// testPair 已启动的一对门面实例（ctx 彼此独立）
type testPair struct {
	srv       Server
	cli       Client
	srvCtx    context.Context
	cliCtx    context.Context
	srvCancel context.CancelFunc
	cliCancel context.CancelFunc
}

// startPairAsync 分别在两个协程中启动 Server 与 Client，上下文隔离
func startPairAsync(t *testing.T, srvProc ServerMessageProcessor, cliProc ClientMessageProcessor) *testPair {
	t.Helper()

	srvCtx, srvCancel := context.WithCancel(context.Background())
	cliCtx, cliCancel := context.WithCancel(context.Background())

	addrCh := make(chan string, 1)
	srvCh := make(chan Server, 1)
	cliCh := make(chan Client, 1)
	errCh := make(chan error, 2)

	t.Log("[启动] goroutine：Server（独立 srvCtx）")
	go func() {
		srv, err := NewServer(ServerConfig{
			Addr:      "127.0.0.1:0",
			Name:      "demo-srv",
			Processor: srvProc,
		})
		if err != nil {
			errCh <- err
			return
		}
		if err := srv.Start(srvCtx); err != nil {
			errCh <- err
			_ = srv.Close(context.Background())
			return
		}
		addr, err := srv.ListenAddr()
		if err != nil {
			errCh <- err
			_ = srv.Close(context.Background())
			return
		}
		t.Logf("[启动] Server 监听 %v", addr)
		addrCh <- addr.String()
		srvCh <- srv
		<-srvCtx.Done()
		_ = srv.Close(context.Background())
	}()

	t.Log("[启动] goroutine：Client（独立 cliCtx）")
	go func() {
		var addr string
		select {
		case addr = <-addrCh:
		case <-time.After(testWait):
			errCh <- context.DeadlineExceeded
			return
		case <-cliCtx.Done():
			return
		}
		cli, err := NewClient(ClientConfig{
			Name:      "demo-cli",
			Processor: cliProc,
		})
		if err != nil {
			errCh <- err
			return
		}
		url := "ws://" + addr + "/"
		t.Logf("[启动] Client.Connect %s", url)
		if err := cli.Connect(cliCtx, url); err != nil {
			errCh <- err
			_ = cli.Close(CloseNormalClosure, "connect failed")
			return
		}
		cliCh <- cli
		<-cliCtx.Done()
		_ = cli.Close(CloseNormalClosure, "test done")
	}()

	pair := &testPair{srvCtx: srvCtx, cliCtx: cliCtx, srvCancel: srvCancel, cliCancel: cliCancel}
	t.Cleanup(func() {
		t.Log("[清理] cancel cliCtx / srvCtx")
		cliCancel()
		srvCancel()
	})

	select {
	case pair.srv = <-srvCh:
	case err := <-errCh:
		t.Fatalf("启动 Server 失败: %v", err)
	case <-time.After(testWait):
		t.Fatal("等待 Server 启动超时")
	}
	select {
	case pair.cli = <-cliCh:
	case err := <-errCh:
		t.Fatalf("启动 Client 失败: %v", err)
	case <-time.After(testWait):
		t.Fatal("等待 Client 连接超时")
	}
	return pair
}

// ---------------------------------------------------------------------------
// 业务 Processor：仅 OnPayload
// ---------------------------------------------------------------------------

// chatServerProcessor 消费 hello，业务内回复 world
type chatServerProcessor struct {
	mu   sync.Mutex
	id   string
	recv []byte
}

func newChatServerProcessor() *chatServerProcessor { return &chatServerProcessor{} }

func (p *chatServerProcessor) OnPayload(ctx SessionContext, payload Payload) error {
	p.mu.Lock()
	p.id = ctx.ConnID()
	p.recv = append([]byte(nil), payload.Data...)
	p.mu.Unlock()
	if bytes.Equal(payload.Data, []byte("hello")) {
		return ctx.Send(MsgText, []byte("world"))
	}
	return nil
}

func (p *chatServerProcessor) connID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.id
}

func (p *chatServerProcessor) received() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.recv...)
}

// chatClientProcessor 消费 world 后完成
type chatClientProcessor struct {
	mu     sync.Mutex
	recv   []byte
	done   chan struct{}
	finish sync.Once
}

func newChatClientProcessor() *chatClientProcessor {
	return &chatClientProcessor{done: make(chan struct{})}
}

func (p *chatClientProcessor) OnPayload(_ PeerContext, payload Payload) error {
	p.mu.Lock()
	p.recv = append([]byte(nil), payload.Data...)
	p.mu.Unlock()
	if bytes.Equal(payload.Data, []byte("world")) {
		p.finish.Do(func() { close(p.done) })
	}
	return nil
}

func (p *chatClientProcessor) received() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.recv...)
}

func (p *chatClientProcessor) waitDone(t *testing.T) {
	t.Helper()
	waitDone(t, p.done, "等待文本往返业务完成超时")
}

// uploadServerProcessor 收到完整载荷后回执 upload-ok
type uploadServerProcessor struct {
	mu     sync.Mutex
	expect []byte
	recv   []byte
}

func newUploadServerProcessor(expect []byte) *uploadServerProcessor {
	return &uploadServerProcessor{expect: expect}
}

func (p *uploadServerProcessor) OnPayload(ctx SessionContext, payload Payload) error {
	p.mu.Lock()
	p.recv = append([]byte(nil), payload.Data...)
	ok := bytes.Equal(p.recv, p.expect)
	p.mu.Unlock()
	if ok {
		return ctx.Send(MsgText, []byte("upload-ok"))
	}
	return nil
}

func (p *uploadServerProcessor) received() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.recv...)
}

// uploadClientProcessor 消费 upload-ok 后完成
type uploadClientProcessor struct {
	mu     sync.Mutex
	ackMsg []byte
	done   chan struct{}
	finish sync.Once
}

func newUploadClientProcessor() *uploadClientProcessor {
	return &uploadClientProcessor{done: make(chan struct{})}
}

func (p *uploadClientProcessor) OnPayload(_ PeerContext, payload Payload) error {
	p.mu.Lock()
	p.ackMsg = append([]byte(nil), payload.Data...)
	p.mu.Unlock()
	if bytes.Equal(payload.Data, []byte("upload-ok")) {
		p.finish.Do(func() { close(p.done) })
	}
	return nil
}

func (p *uploadClientProcessor) ack() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.ackMsg...)
}

func (p *uploadClientProcessor) waitDone(t *testing.T) {
	t.Helper()
	waitDone(t, p.done, "等待上传业务完成超时")
}

// downloadServerProcessor 消费 GET_FILE 后整包下发
type downloadServerProcessor struct {
	mu   sync.Mutex
	file []byte
	req  []byte
}

func newDownloadServerProcessor(file []byte) *downloadServerProcessor {
	return &downloadServerProcessor{file: file}
}

func (p *downloadServerProcessor) OnPayload(ctx SessionContext, payload Payload) error {
	p.mu.Lock()
	p.req = append([]byte(nil), payload.Data...)
	p.mu.Unlock()
	if bytes.Equal(payload.Data, []byte("GET_FILE")) {
		return ctx.Send(MsgBinary, p.file)
	}
	return nil
}

func (p *downloadServerProcessor) request() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.req...)
}

// downloadClientProcessor 收到完整下发后完成
type downloadClientProcessor struct {
	mu     sync.Mutex
	expect []byte
	recv   []byte
	done   chan struct{}
	finish sync.Once
}

func newDownloadClientProcessor(expect []byte) *downloadClientProcessor {
	return &downloadClientProcessor{expect: expect, done: make(chan struct{})}
}

func (p *downloadClientProcessor) OnPayload(_ PeerContext, payload Payload) error {
	p.mu.Lock()
	p.recv = append([]byte(nil), payload.Data...)
	ok := bytes.Equal(p.recv, p.expect)
	p.mu.Unlock()
	if ok {
		p.finish.Do(func() { close(p.done) })
	}
	return nil
}

func (p *downloadClientProcessor) received() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.recv...)
}

func (p *downloadClientProcessor) waitDone(t *testing.T) {
	t.Helper()
	waitDone(t, p.done, "等待下发业务完成超时")
}

func waitDone(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(testWait):
		t.Fatal(msg)
	}
}

// TestClientConnectedClearsOnPeerClose 服务端关闭会话后客户端 Connected 应变为 false
func TestClientConnectedClearsOnPeerClose(t *testing.T) {
	t.Log("[用例] 对端断开后 Connected 应清空")

	srvProc := newChatServerProcessor()
	cliProc := newChatClientProcessor()
	pair := startPairAsync(t, srvProc, cliProc)

	if !pair.cli.Connected() {
		t.Fatal("连通后 Connected 应为 true")
	}

	t.Log("[流程] 服务端 Close（停听并 CloseAll）")
	if err := pair.srv.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		if !pair.cli.Connected() {
			t.Log("[断言] Connected 已变为 false")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("对端关闭后 Connected 仍为 true")
}

// TestServerCloseLeavesHubEmpty Close 后 Hub 应为空，并发拨号不得再登记
func TestServerCloseLeavesHubEmpty(t *testing.T) {
	t.Log("[用例] Close 后 Len=0，关闭窗口内新连接不入 Hub")

	srvCtx, srvCancel := context.WithCancel(context.Background())
	defer srvCancel()

	srv, err := NewServer(ServerConfig{
		Addr:      "127.0.0.1:0",
		Name:      "close-hub",
		Processor: newChatServerProcessor(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(srvCtx); err != nil {
		t.Fatal(err)
	}
	addr, err := srv.ListenAddr()
	if err != nil {
		t.Fatal(err)
	}
	url := "ws://" + addr.String() + "/"

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cli, err := NewClient(ClientConfig{
				Name:      "close-racer",
				Processor: newChatClientProcessor(),
			})
			if err != nil {
				return
			}
			_ = cli.Connect(context.Background(), url)
			_ = cli.Close(CloseNormalClosure, "done")
		}()
	}

	time.Sleep(20 * time.Millisecond)
	t.Log("[流程] Close 与并发 Connect 交错")
	if err := srv.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		if srv.Len() == 0 {
			t.Log("[断言] Close 后 Hub 为空")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Close 后 Len=%d，期望 0", srv.Len())
}

// TestServerSendUnknownPeer 未知 ConnID 应返回 ErrUnknownPeer
func TestServerSendUnknownPeer(t *testing.T) {
	pair := startPairAsync(t, newChatServerProcessor(), newChatClientProcessor())
	err := pair.srv.Send("missing", MsgText, []byte("x"))
	if !errors.Is(err, ErrUnknownPeer) {
		t.Fatalf("应得 ErrUnknownPeer，实际 %v", err)
	}
}

// TestClientSendAfterClose 关闭后 Send 应返回 ErrNotConnected
func TestClientSendAfterClose(t *testing.T) {
	pair := startPairAsync(t, newChatServerProcessor(), newChatClientProcessor())
	if err := pair.cli.Close(CloseNormalClosure, "bye"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, testWait, func() bool { return !pair.cli.Connected() }, "等待客户端断开超时")
	err := pair.cli.Send(MsgText, []byte("x"))
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("应得 ErrNotConnected，实际 %v", err)
	}
}

// TestServerSendAfterClose 服务关闭后 Send 应返回 ErrClosed
func TestServerSendAfterClose(t *testing.T) {
	pair := startPairAsync(t, newChatServerProcessor(), newChatClientProcessor())
	if err := pair.srv.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := pair.srv.Send("any", MsgText, []byte("x"))
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("应得 ErrClosed，实际 %v", err)
	}
}

// TestSessionEmptyOnConnect 框架不向 Session 写入内部键
func TestSessionEmptyOnConnect(t *testing.T) {
	h := &sessionProbeHandler{ready: make(chan int, 1)}
	_, url := startTestServer(t, ServerConfig{
		Addr:             "127.0.0.1:0",
		HeartbeatTimeout: Duration(0),
		MaxConnLifetime:  Duration(0),
		Handler:          h,
	})
	_ = connectTestClient(t, url, ClientConfig{
		HeartbeatInterval: Duration(0),
		HeartbeatTimeout:  Duration(0),
		Handler:           NopClientHandler{},
	})
	select {
	case n := <-h.ready:
		if n != 0 {
			t.Fatalf("OnConnect 时 Session.Len=%d，期望 0", n)
		}
	case <-time.After(testWait):
		t.Fatal("等待 OnConnect 超时")
	}
}

// TestMaxConnectionsRejectsExtra 超额连接握手后断开，不占 Hub
func TestMaxConnectionsRejectsExtra(t *testing.T) {
	t.Log("[用例] MaxConnections=1，第二条应被 CloseTryAgainLater")

	extraH := newDisconnectClientHandler()
	srv, url := startTestServer(t, ServerConfig{
		Addr:             "127.0.0.1:0",
		MaxConnections:   1,
		HeartbeatTimeout: Duration(0),
		MaxConnLifetime:  Duration(0),
		Handler:          NopServerHandler{},
	})
	first := connectTestClient(t, url, ClientConfig{
		HeartbeatInterval: Duration(0),
		HeartbeatTimeout:  Duration(0),
		Handler:           NopClientHandler{},
	})
	waitUntil(t, testWait, func() bool { return srv.Len() == 1 && first.Connected() }, "等待第一条登记超时")

	extra := connectTestClient(t, url, ClientConfig{
		HeartbeatInterval: Duration(0),
		HeartbeatTimeout:  Duration(0),
		Handler:           extraH,
	})
	extraH.wait(t)
	waitUntil(t, testWait, func() bool { return !extra.Connected() }, "超额客户端应断开")
	if srv.Len() != 1 {
		t.Fatalf("Len=%d want 1", srv.Len())
	}
	if !first.Connected() {
		t.Fatal("既有连接不应被踢")
	}
	t.Log("[断言] Hub 仍为 1，超额客户端已断开")
}

// TestServerSendByConnID 按框架 ConnID 推送；自定义 Handler 嵌入 DefaultServerHandler 以接入 Processor
func TestServerSendByConnID(t *testing.T) {
	idCh := make(chan string, 1)
	cliProc := newChatClientProcessor()
	h := &captureIDHandler{
		DefaultServerHandler: DefaultServerHandler{Processor: newChatServerProcessor()},
		idCh:                 idCh,
	}
	srv, url := startTestServer(t, ServerConfig{
		Addr:             "127.0.0.1:0",
		HeartbeatTimeout: Duration(0),
		MaxConnLifetime:  Duration(0),
		Handler:          h,
	})
	_ = connectTestClient(t, url, ClientConfig{
		HeartbeatInterval: Duration(0),
		HeartbeatTimeout:  Duration(0),
		Processor:         cliProc,
	})

	var id string
	select {
	case id = <-idCh:
	case <-time.After(testWait):
		t.Fatal("等待 ConnID 超时")
	}
	if !srv.Has(id) {
		t.Fatalf("Has(%s)=false", id)
	}
	if err := srv.Send(id, MsgText, []byte("world")); err != nil {
		t.Fatal(err)
	}
	cliProc.waitDone(t)
}

// sessionProbeHandler 在 OnConnect 报告 Session 条目数
type sessionProbeHandler struct {
	ready chan int
}

func (h *sessionProbeHandler) OnConnect(ctx SessionContext) {
	select {
	case h.ready <- ctx.Session().Len():
	default:
	}
}

func (h *sessionProbeHandler) OnDisconnect(string, error) {}

func (h *sessionProbeHandler) OnPayload(SessionContext, Payload) {}

// captureIDHandler 嵌入默认 Handler，在 OnConnect 暴露 ConnID
type captureIDHandler struct {
	DefaultServerHandler
	idCh chan string
}

func (h *captureIDHandler) OnConnect(ctx SessionContext) {
	h.DefaultServerHandler.OnConnect(ctx)
	select {
	case h.idCh <- ctx.ConnID():
	default:
	}
}
