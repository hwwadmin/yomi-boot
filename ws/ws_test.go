// Package ws 门面联调测试：演示如何定义 Processor、配置启动 Server/Client、收发消息与分片发文件
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

// TestProcessorConnectAndText 定义 Processor → 配置启动 → 文本往返
func TestProcessorConnectAndText(t *testing.T) {
	t.Log("[用例] Processor 主路径：连通 + 文本收发")

	srvProc := newRecordServerProcessor()
	cliProc := newRecordClientProcessor()
	srv, cli := startPair(t, srvProc, cliProc, 0)

	connID := srvProc.waitReady(t)
	cliProc.waitReady(t)
	t.Logf("[断言] 已连通 connID=%s Client.Connected=%v", connID, cli.Connected())
	if !cli.Connected() || !srv.Has(connID) {
		t.Fatal("连接状态异常")
	}

	t.Log("[发送] Client.SendText hello")
	if err := cli.SendText(context.Background(), []byte("hello")); err != nil {
		t.Fatal(err)
	}
	got := srvProc.waitPayloads(t, 1)
	if !bytes.Equal(got[0].Data, []byte("hello")) {
		t.Fatalf("服务端收到=%q", got[0].Data)
	}
	t.Log("[断言] 服务端 Processor 收到 hello")

	t.Logf("[发送] Server.SendText(connID=%s) world", connID)
	if err := srv.SendText(connID, []byte("world")); err != nil {
		t.Fatal(err)
	}
	got = cliProc.waitPayloads(t, 1)
	if !bytes.Equal(got[0].Data, []byte("world")) {
		t.Fatalf("客户端收到=%q", got[0].Data)
	}
	t.Log("[断言] 客户端 Processor 收到 world（客户端不感知 ConnID）")
}

// TestProcessorSendFileChunks 分片发文件：Client.SendReader → 服务端多片 OnPayload 拼回
func TestProcessorSendFileChunks(t *testing.T) {
	const chunk = 4
	file := []byte("abcdefghijkl") // 12 字节 → 3 片
	t.Logf("[用例] 分片发文件 chunk=%d 总长=%d 期望片数=%d", chunk, len(file), (len(file)+chunk-1)/chunk)

	srvProc := newRecordServerProcessor()
	cliProc := newRecordClientProcessor()
	_, cli := startPair(t, srvProc, cliProc, chunk)

	_ = srvProc.waitReady(t)
	cliProc.waitReady(t)

	t.Log("[发送] Client.SendReader 模拟上传文件")
	if err := cli.SendReader(context.Background(), OpcodeBinary, bytes.NewReader(file), WithChunkSize(chunk)); err != nil {
		t.Fatal(err)
	}
	parts := srvProc.waitPayloads(t, 3)
	joined := joinPayloadData(parts)
	if !bytes.Equal(joined, file) {
		t.Fatalf("拼接=%q want %q", joined, file)
	}
	t.Logf("[断言] 服务端收到 %d 片，拼接与原文一致", len(parts))
}

// TestProcessorServerPushFile 服务端按 ConnID 分片下发文件
func TestProcessorServerPushFile(t *testing.T) {
	const chunk = 5
	file := []byte("hello-file!!") // 12 字节 → 3 片（5+5+2）
	t.Logf("[用例] 服务端分片下发 chunk=%d 总长=%d", chunk, len(file))

	srvProc := newRecordServerProcessor()
	cliProc := newRecordClientProcessor()
	srv, _ := startPair(t, srvProc, cliProc, chunk)

	connID := srvProc.waitReady(t)
	cliProc.waitReady(t)

	t.Logf("[发送] Server.SendReader(connID=%s)", connID)
	if err := srv.SendReader(connID, OpcodeBinary, bytes.NewReader(file), WithChunkSize(chunk)); err != nil {
		t.Fatal(err)
	}
	parts := cliProc.waitPayloads(t, 3)
	joined := joinPayloadData(parts)
	if !bytes.Equal(joined, file) {
		t.Fatalf("拼接=%q want %q", joined, file)
	}
	t.Logf("[断言] 客户端收到 %d 片，拼接与原文一致", len(parts))
}

// ---------------------------------------------------------------------------
// 辅助：可观察 Processor + 启动装配
// ---------------------------------------------------------------------------

// recordServerProcessor 服务端处理器：记录 ConnID 与收到的载荷
type recordServerProcessor struct {
	mu       sync.Mutex
	connID   string
	payloads []Payload

	ready  chan struct{}
	gotMsg chan struct{}
}

// newRecordServerProcessor 创建可观察的服务端处理器
func newRecordServerProcessor() *recordServerProcessor {
	return &recordServerProcessor{
		ready:  make(chan struct{}, 1),
		gotMsg: make(chan struct{}, 64),
	}
}

// OnConnect 连接建立后记录框架 ConnID
func (p *recordServerProcessor) OnConnect(ctx SessionContext) {
	p.mu.Lock()
	p.connID = ctx.ConnID()
	p.mu.Unlock()
	tlogReady(p.ready)
}

// OnDisconnect 连接结束（本示例空实现）
func (p *recordServerProcessor) OnDisconnect(string, error) {}

// OnPayload 消费一条业务消息
func (p *recordServerProcessor) OnPayload(_ SessionContext, payload Payload) error {
	p.mu.Lock()
	p.payloads = append(p.payloads, payload)
	p.mu.Unlock()
	tlogReady(p.gotMsg)
	return nil
}

// waitReady 等待 OnConnect
func (p *recordServerProcessor) waitReady(t *testing.T) string {
	t.Helper()
	select {
	case <-p.ready:
	case <-time.After(testWait):
		t.Fatal("等待服务端 Processor OnConnect 超时")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.connID == "" {
		t.Fatal("ConnID 为空")
	}
	return p.connID
}

// waitPayloads 等待至少 n 条载荷
func (p *recordServerProcessor) waitPayloads(t *testing.T, n int) []Payload {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-p.gotMsg:
		case <-time.After(testWait):
			t.Fatalf("等待服务端 OnPayload 超时 at %d/%d", i, n)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Payload, len(p.payloads))
	copy(out, p.payloads)
	return out
}

// recordClientProcessor 客户端处理器：记录收到的载荷（无 ConnID）
type recordClientProcessor struct {
	mu       sync.Mutex
	payloads []Payload

	ready  chan struct{}
	gotMsg chan struct{}
}

// newRecordClientProcessor 创建可观察的客户端处理器
func newRecordClientProcessor() *recordClientProcessor {
	return &recordClientProcessor{
		ready:  make(chan struct{}, 1),
		gotMsg: make(chan struct{}, 64),
	}
}

// OnConnect 出站已连通
func (p *recordClientProcessor) OnConnect(PeerContext) {
	tlogReady(p.ready)
}

// OnDisconnect 连接结束（本示例空实现）
func (p *recordClientProcessor) OnDisconnect(error) {}

// OnPayload 消费一条业务消息
func (p *recordClientProcessor) OnPayload(_ PeerContext, payload Payload) error {
	p.mu.Lock()
	p.payloads = append(p.payloads, payload)
	p.mu.Unlock()
	tlogReady(p.gotMsg)
	return nil
}

// waitReady 等待 OnConnect
func (p *recordClientProcessor) waitReady(t *testing.T) {
	t.Helper()
	select {
	case <-p.ready:
	case <-time.After(testWait):
		t.Fatal("等待客户端 Processor OnConnect 超时")
	}
}

// waitPayloads 等待至少 n 条载荷
func (p *recordClientProcessor) waitPayloads(t *testing.T, n int) []Payload {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-p.gotMsg:
		case <-time.After(testWait):
			t.Fatalf("等待客户端 OnPayload 超时 at %d/%d", i, n)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Payload, len(p.payloads))
	copy(out, p.payloads)
	return out
}

func tlogReady(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// startPair 按业务主路径启动一对 Server/Client（只配 Processor）
func startPair(t *testing.T, srvProc ServerMessageProcessor, cliProc ClientMessageProcessor, chunkSize int) (Server, Client) {
	t.Helper()
	t.Log("[配置] NewServer：Addr=127.0.0.1:0，Processor=业务实现，Handler 默认")

	srv, err := NewServer(ServerConfig{
		ServerConfig: wsbase.ServerConfig{
			Addr:      "127.0.0.1:0",
			Name:      "demo-srv",
			ChunkSize: chunkSize,
		},
		Processor: srvProc,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		t.Log("[清理] 关闭 Server")
		cancel()
		_ = srv.Close(context.Background())
	})

	t.Log("[启动] Server.Start")
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr, err := srv.ListenAddr()
	if err != nil {
		t.Fatalf("ListenAddr: %v", err)
	}
	t.Logf("[启动] Server 监听 %v", addr)

	t.Log("[配置] NewClient：Processor=业务实现，Handler 默认")
	cli, err := NewClient(ClientConfig{
		ClientConfig: wsbase.ClientConfig{
			Name:      "demo-cli",
			ChunkSize: chunkSize,
		},
		Processor: cliProc,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() {
		t.Log("[清理] 关闭 Client")
		_ = cli.Close(CloseNormalClosure, "test done")
	})

	url := "ws://" + addr.String() + "/"
	t.Logf("[启动] Client.Connect %s", url)
	if err := cli.Connect(context.Background(), url); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return srv, cli
}

func joinPayloadData(ps []Payload) []byte {
	var b []byte
	for _, p := range ps {
		b = append(b, p.Data...)
	}
	return b
}
