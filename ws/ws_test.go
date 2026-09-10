// Package ws 门面联调测试：演示 Processor 只消费消息；连通后主动发信在 Connect 之后发起
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

// TestProcessorConnectAndText 连通后客户端发 hello；服务端 OnPayload 回 world；客户端 OnPayload 收妥
func TestProcessorConnectAndText(t *testing.T) {
	t.Log("[用例] 文本往返（Processor 只负责 OnPayload）")

	srvProc := newChatServerProcessor()
	cliProc := newChatClientProcessor()
	pair := startPairAsync(t, srvProc, cliProc)

	t.Log("[发送] Connect 成功后 Client.Send MsgText hello")
	if err := pair.cli.Send(pair.cliCtx, MsgText, []byte("hello")); err != nil {
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
	if err := pair.cli.Send(pair.cliCtx, MsgBinary, file); err != nil {
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
	if err := pair.cli.Send(pair.cliCtx, MsgText, []byte("GET_FILE")); err != nil {
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
			ServerConfig: wsbase.ServerConfig{
				Addr: "127.0.0.1:0",
				Name: "demo-srv",
			},
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
			ClientConfig: wsbase.ClientConfig{
				Name: "demo-cli",
			},
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
