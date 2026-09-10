// Package wsapp 的 Server/Client 门面联调测试
//
// @author zwei
package wsapp

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/ws"
)

const testWait = 2 * time.Second

// testServerBiz 可观察的服务端业务 Handler
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
func (b *testServerBiz) OnMessage(ctx SessionContext, msg ws.Message) {
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

// testClientBiz 可观察的客户端业务 Handler（无 ConnID）
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
func (b *testClientBiz) OnMessage(_ PeerContext, msg ws.Message) {
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

// startAppPair 启动 app.Server，Client 直连（不传任何会话身份）
func startAppPair(t *testing.T, srvBiz ServerHandler, cliBiz ClientHandler) (Server, Client, string) {
	t.Helper()
	t.Log("[流程] 启动 app.Server（ConnID 由服务端自动生成）")

	srv, err := NewServer(srvBiz, ServerConfig{
		ServerConfig: ws.ServerConfig{Addr: "127.0.0.1:0", Name: "app-srv"},
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

	cli, err := NewClient(cliBiz, ClientConfig{
		ClientConfig: ws.ClientConfig{Name: "app-cli"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	url := "ws://" + addr.String() + "/"
	t.Logf("[流程] Client Connect url=%s（客户端不携带 ConnID）", url)
	if err := cli.Connect(context.Background(), url); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close(ws.CloseNormalClosure, "test done") })

	var connID string
	if tb, ok := srvBiz.(*testServerBiz); ok {
		connID = tb.lastConnID(t)
		t.Logf("[流程] 服务端业务 OnConnect connID=%s", connID)
	}
	if tb, ok := cliBiz.(*testClientBiz); ok {
		tb.waitConnect(t)
		t.Log("[流程] 客户端业务 OnConnect（无 ConnID）")
	}
	return srv, cli, connID
}

// TestAppServerClientConnect 门面基本用例：连通后服务端拥有 ConnID
func TestAppServerClientConnect(t *testing.T) {
	t.Log("[用例] app.Server + app.Client 连接")
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
	t.Log("[用例] 门面文本往返")
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
		t.Fatalf("服务端业务收到=%q", got)
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
		t.Fatalf("客户端业务收到=%q", got)
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
	if err := cli.SendReader(context.Background(), ws.OpcodeBinary, bytes.NewReader(payload), ws.WithChunkSize(chunk)); err != nil {
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
	srv, err := NewServer(NopServerHandler{}, ServerConfig{
		ServerConfig: ws.ServerConfig{Addr: "127.0.0.1:0"},
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
	cli, err := NewClient(NopClientHandler{}, ClientConfig{})
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
