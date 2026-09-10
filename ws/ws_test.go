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
	pair := startPairAsync(t, srvProc, cliProc, 0)

	t.Log("[发送] Connect 成功后 Client.SendText hello")
	if err := pair.cli.SendText(pair.cliCtx, []byte("hello")); err != nil {
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

// TestProcessorSendFileChunks 连通后客户端分片上传；服务端 OnPayload 拼齐回执
func TestProcessorSendFileChunks(t *testing.T) {
	const chunk = 4
	file := []byte("abcdefghijkl")
	t.Logf("[用例] 分片上传 chunk=%d 总长=%d", chunk, len(file))

	srvProc := newUploadServerProcessor(file)
	cliProc := newUploadClientProcessor()
	pair := startPairAsync(t, srvProc, cliProc, chunk)

	t.Log("[发送] Connect 成功后 Client.SendReader")
	if err := pair.cli.SendReader(pair.cliCtx, OpcodeBinary, bytes.NewReader(file), WithChunkSize(chunk)); err != nil {
		t.Fatal(err)
	}

	cliProc.waitDone(t)
	if !bytes.Equal(srvProc.joined(), file) {
		t.Fatalf("服务端拼接=%q want %q", srvProc.joined(), file)
	}
	if !bytes.Equal(cliProc.ack(), []byte("upload-ok")) {
		t.Fatalf("客户端回执=%q want upload-ok", cliProc.ack())
	}
	t.Logf("[断言] 服务端 OnPayload 拼回 %d 片并回执", srvProc.partCount())
}

// TestProcessorServerPushFile 连通后客户端发 GET_FILE；服务端 OnPayload 内下发文件
func TestProcessorServerPushFile(t *testing.T) {
	const chunk = 5
	file := []byte("hello-file!!")
	t.Logf("[用例] 请求下发 chunk=%d 总长=%d", chunk, len(file))

	srvProc := newDownloadServerProcessor(file, chunk)
	cliProc := newDownloadClientProcessor(file)
	pair := startPairAsync(t, srvProc, cliProc, chunk)

	t.Log("[发送] Connect 成功后 Client.SendText GET_FILE")
	if err := pair.cli.SendText(pair.cliCtx, []byte("GET_FILE")); err != nil {
		t.Fatal(err)
	}

	cliProc.waitDone(t)
	if !bytes.Equal(srvProc.request(), []byte("GET_FILE")) {
		t.Fatalf("服务端请求=%q want GET_FILE", srvProc.request())
	}
	if !bytes.Equal(cliProc.joined(), file) {
		t.Fatalf("客户端拼接=%q want %q", cliProc.joined(), file)
	}
	t.Logf("[断言] 服务端 OnPayload 下发；客户端拼回 %d 片", cliProc.partCount())
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
func startPairAsync(t *testing.T, srvProc ServerMessageProcessor, cliProc ClientMessageProcessor, chunkSize int) *testPair {
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
				Addr:      "127.0.0.1:0",
				Name:      "demo-srv",
				ChunkSize: chunkSize,
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
		t.Log("[清理] Server goroutine 退出，Close")
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
				Name:      "demo-cli",
				ChunkSize: chunkSize,
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
		t.Log("[清理] Client goroutine 退出，Close")
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
		return ctx.SendText([]byte("world"))
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

// uploadServerProcessor 拼齐期望文件后回执 upload-ok
type uploadServerProcessor struct {
	mu     sync.Mutex
	expect []byte
	parts  [][]byte
}

func newUploadServerProcessor(expect []byte) *uploadServerProcessor {
	return &uploadServerProcessor{expect: expect}
}

func (p *uploadServerProcessor) OnPayload(ctx SessionContext, payload Payload) error {
	p.mu.Lock()
	p.parts = append(p.parts, append([]byte(nil), payload.Data...))
	joined := bytes.Join(p.parts, nil)
	done := bytes.Equal(joined, p.expect)
	p.mu.Unlock()
	if done {
		return ctx.SendText([]byte("upload-ok"))
	}
	return nil
}

func (p *uploadServerProcessor) joined() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return bytes.Join(p.parts, nil)
}

func (p *uploadServerProcessor) partCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.parts)
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

// downloadServerProcessor 消费 GET_FILE 后分片下发
type downloadServerProcessor struct {
	mu    sync.Mutex
	file  []byte
	chunk int
	req   []byte
}

func newDownloadServerProcessor(file []byte, chunk int) *downloadServerProcessor {
	return &downloadServerProcessor{file: file, chunk: chunk}
}

func (p *downloadServerProcessor) OnPayload(ctx SessionContext, payload Payload) error {
	p.mu.Lock()
	p.req = append([]byte(nil), payload.Data...)
	p.mu.Unlock()
	if bytes.Equal(payload.Data, []byte("GET_FILE")) {
		return ctx.SendReader(OpcodeBinary, bytes.NewReader(p.file), WithChunkSize(p.chunk))
	}
	return nil
}

func (p *downloadServerProcessor) request() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.req...)
}

// downloadClientProcessor 拼齐下发文件后完成
type downloadClientProcessor struct {
	mu     sync.Mutex
	expect []byte
	parts  [][]byte
	done   chan struct{}
	finish sync.Once
}

func newDownloadClientProcessor(expect []byte) *downloadClientProcessor {
	return &downloadClientProcessor{expect: expect, done: make(chan struct{})}
}

func (p *downloadClientProcessor) OnPayload(_ PeerContext, payload Payload) error {
	p.mu.Lock()
	p.parts = append(p.parts, append([]byte(nil), payload.Data...))
	joined := bytes.Join(p.parts, nil)
	done := bytes.Equal(joined, p.expect)
	p.mu.Unlock()
	if done {
		p.finish.Do(func() { close(p.done) })
	}
	return nil
}

func (p *downloadClientProcessor) joined() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return bytes.Join(p.parts, nil)
}

func (p *downloadClientProcessor) partCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.parts)
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
