// Package ws 的 Server/Client 联调测试
//
// @author zwei
package ws

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

const testWait = 2 * time.Second

// testEnv 表示已启动的 Server 与已连上的 Client 连接
type testEnv struct {
	t          *testing.T
	srv        *Server
	client     *Client
	clientConn Conn
	serverConn Conn // 服务端侧连接，由 srv OnOpen 捕获
	cancel     context.CancelFunc

	serverOpen  chan Conn
	clientOpen  chan Conn
	serverMsgs  chan []byte
	clientMsgs  chan []byte
	serverClose chan struct{}
	clientClose chan struct{}
}

// startServerAndClient 打开独立端口 Server，并用 Client Dial 连上；调用方负责 env.close()
func startServerAndClient(t *testing.T, srvCfg ServerConfig, cliCfg ClientConfig) *testEnv {
	t.Helper()
	if srvCfg.Addr == "" {
		srvCfg.Addr = "127.0.0.1:0"
	}

	env := &testEnv{
		t:           t,
		serverOpen:  make(chan Conn, 1),
		clientOpen:  make(chan Conn, 1),
		serverMsgs:  make(chan []byte, 64),
		clientMsgs:  make(chan []byte, 64),
		serverClose: make(chan struct{}, 1),
		clientClose: make(chan struct{}, 1),
	}

	srvHandler := &testHandler{
		onOpen: func(c Conn) {
			select {
			case env.serverOpen <- c:
			default:
			}
		},
		onMessage: func(_ Conn, msg Message) {
			defer msg.Close()
			cp := append([]byte(nil), msg.Bytes()...)
			select {
			case env.serverMsgs <- cp:
			default:
				t.Error("serverMsgs 通道已满")
			}
		},
		onClose: func(Conn, error) {
			select {
			case env.serverClose <- struct{}{}:
			default:
			}
		},
	}
	cliHandler := &testHandler{
		onOpen: func(c Conn) {
			select {
			case env.clientOpen <- c:
			default:
			}
		},
		onMessage: func(_ Conn, msg Message) {
			defer msg.Close()
			cp := append([]byte(nil), msg.Bytes()...)
			select {
			case env.clientMsgs <- cp:
			default:
				t.Error("clientMsgs 通道已满")
			}
		},
		onClose: func(Conn, error) {
			select {
			case env.clientClose <- struct{}{}:
			default:
			}
		},
	}

	srv, err := NewServer(srvHandler, srvCfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	env.cancel = cancel
	if err := srv.Start(ctx); err != nil {
		cancel()
		t.Fatalf("Server.Start: %v", err)
	}
	env.srv = srv

	addr, err := srv.ListenAddr()
	if err != nil {
		env.close()
		t.Fatalf("ListenAddr: %v", err)
	}

	cli, err := NewClient(cliHandler, cliCfg)
	if err != nil {
		env.close()
		t.Fatalf("NewClient: %v", err)
	}
	env.client = cli

	conn, _, err := cli.Dial(context.Background(), "ws://"+addr.String()+"/")
	if err != nil {
		env.close()
		t.Fatalf("Client.Dial: %v", err)
	}
	env.clientConn = conn
	go conn.Serve()

	select {
	case env.serverConn = <-env.serverOpen:
	case <-time.After(testWait):
		env.close()
		t.Fatal("等待服务端 OnOpen 超时")
	}
	select {
	case <-env.clientOpen:
	case <-time.After(testWait):
		env.close()
		t.Fatal("等待客户端 OnOpen 超时")
	}
	return env
}

// close 关闭客户端连接与 Server
func (env *testEnv) close() {
	if env.clientConn != nil {
		_ = env.clientConn.Close(CloseNormalClosure, "test done")
	}
	if env.cancel != nil {
		env.cancel()
	}
	if env.srv != nil {
		_ = env.srv.Close(context.Background())
	}
}

// waitServerMsg 等待服务端收到一条消息
func (env *testEnv) waitServerMsg() []byte {
	env.t.Helper()
	select {
	case b := <-env.serverMsgs:
		return b
	case <-time.After(testWait):
		env.t.Fatal("等待服务端消息超时")
		return nil
	}
}

// waitClientMsg 等待客户端收到一条消息
func (env *testEnv) waitClientMsg() []byte {
	env.t.Helper()
	select {
	case b := <-env.clientMsgs:
		return b
	case <-time.After(testWait):
		env.t.Fatal("等待客户端消息超时")
		return nil
	}
}

// TestServerAndClientConnect 基本用例：启动 Server，Client 拨号连上，双方 OnOpen
func TestServerAndClientConnect(t *testing.T) {
	env := startServerAndClient(t, ServerConfig{Name: "srv"}, ClientConfig{Name: "cli"})
	defer env.close()

	if env.srv.Name() != "srv" {
		t.Fatalf("Server.Name=%q", env.srv.Name())
	}
	if env.client.Name() != "cli" {
		t.Fatalf("Client.Name=%q", env.client.Name())
	}
	if env.serverConn == nil || env.clientConn == nil {
		t.Fatal("两侧连接均应非空")
	}
	if env.clientConn.RemoteAddr() == nil || env.serverConn.RemoteAddr() == nil {
		t.Fatal("RemoteAddr 不应为 nil")
	}
}

// TestServerClientTextEcho 客户端发文本，服务端收到后再回写，客户端收到
func TestServerClientTextEcho(t *testing.T) {
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	want := []byte("hello-ws")
	if err := env.clientConn.WriteText(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got := env.waitServerMsg()
	if !bytes.Equal(got, want) {
		t.Fatalf("服务端收到=%q, want %q", got, want)
	}

	reply := []byte("pong-text")
	if err := env.serverConn.WriteText(context.Background(), reply); err != nil {
		t.Fatal(err)
	}
	got = env.waitClientMsg()
	if !bytes.Equal(got, reply) {
		t.Fatalf("客户端收到=%q, want %q", got, reply)
	}
}

// TestServerClientBinaryEcho 客户端发二进制，服务端收到后再回写
func TestServerClientBinaryEcho(t *testing.T) {
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	want := []byte{0x00, 0x01, 0xff, 0xfe}
	if err := env.clientConn.WriteBinary(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got := env.waitServerMsg()
	if !bytes.Equal(got, want) {
		t.Fatalf("服务端收到=%v, want %v", got, want)
	}

	if err := env.serverConn.WriteBinary(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got = env.waitClientMsg()
	if !bytes.Equal(got, want) {
		t.Fatalf("客户端收到=%v, want %v", got, want)
	}
}

// TestServerClientChunkedUpload 客户端按可配置分片上传，服务端按片接收
func TestServerClientChunkedUpload(t *testing.T) {
	const chunk = 4
	env := startServerAndClient(t,
		ServerConfig{ChunkSize: chunk},
		ClientConfig{ChunkSize: chunk},
	)
	defer env.close()

	payload := []byte("abcdefghijkl") // 12 → 3 片
	if err := env.clientConn.WriteReader(context.Background(), OpcodeBinary, bytes.NewReader(payload), WithChunkSize(chunk)); err != nil {
		t.Fatal(err)
	}

	var parts [][]byte
	for i := 0; i < 3; i++ {
		parts = append(parts, env.waitServerMsg())
	}
	if joined := bytes.Join(parts, nil); !bytes.Equal(joined, payload) {
		t.Fatalf("拼接=%q, want %q", joined, payload)
	}
	for i, p := range parts {
		if len(p) != chunk {
			t.Fatalf("分片[%d] len=%d, want %d", i, len(p), chunk)
		}
	}
}

// TestServerClientChunkedDownload 服务端按分片下发，客户端按片接收
func TestServerClientChunkedDownload(t *testing.T) {
	const chunk = 5
	env := startServerAndClient(t,
		ServerConfig{ChunkSize: chunk},
		ClientConfig{ChunkSize: chunk},
	)
	defer env.close()

	payload := []byte("0123456789") // 10 → 2 片
	if err := env.serverConn.WriteReader(context.Background(), OpcodeBinary, bytes.NewReader(payload), WithChunkSize(chunk)); err != nil {
		t.Fatal(err)
	}

	var parts [][]byte
	for i := 0; i < 2; i++ {
		parts = append(parts, env.waitClientMsg())
	}
	if joined := bytes.Join(parts, nil); !bytes.Equal(joined, payload) {
		t.Fatalf("拼接=%q, want %q", joined, payload)
	}
}

// TestServerClientWriteReaderCanceledContext 已取消 ctx 时客户端 WriteReader 失败
func TestServerClientWriteReaderCanceledContext(t *testing.T) {
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	writeCtx, writeCancel := context.WithCancel(context.Background())
	writeCancel()
	err := env.clientConn.WriteReader(writeCtx, OpcodeBinary, strings.NewReader(strings.Repeat("x", 1024)), WithChunkSize(64))
	if err == nil {
		t.Fatal("已取消 ctx 应返回错误")
	}
}

// TestServerClientNative 验证连上后 Native 逃生舱可用
func TestServerClientNative(t *testing.T) {
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	n, ok := env.clientConn.(NativeConn)
	if !ok || n.Native() == nil {
		t.Fatal("客户端 Conn 应支持 Native()")
	}
	n2, ok := env.serverConn.(NativeConn)
	if !ok || n2.Native() == nil {
		t.Fatal("服务端 Conn 应支持 Native()")
	}
	if env.srv.NativeUpgrader() == nil {
		t.Fatal("NativeUpgrader 不应为 nil")
	}
}

// TestServerClientHub 连上后将服务端连接注册进 Hub 并可取出
func TestServerClientHub(t *testing.T) {
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	hub := NewHub()
	hub.Register("u1", env.serverConn)
	got, ok := hub.Get("u1")
	if !ok || got != env.serverConn {
		t.Fatal("Hub.Get 失败")
	}
	if hub.Len() != 1 {
		t.Fatalf("Len=%d", hub.Len())
	}
	hub.Unregister("u1", env.serverConn)
	if hub.Len() != 0 {
		t.Fatal("Unregister 后应为空")
	}
}

// TestServerConfigValidate 校验 Addr/ChunkSize 边界（无网络）
func TestServerConfigValidate(t *testing.T) {
	if err := (ServerConfig{}).Validate(); err == nil {
		t.Fatal("空 Addr 应失败")
	}
	if err := (ServerConfig{Addr: ":0", ChunkSize: -1}).Validate(); err == nil {
		t.Fatal("负 ChunkSize 应失败")
	}
	if err := (ServerConfig{Addr: ":0", ChunkSize: MaxChunkSize + 1}).Validate(); err == nil {
		t.Fatal("超大 ChunkSize 应失败")
	}
	if err := (ServerConfig{Addr: ":0"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if got := (ServerConfig{Addr: ":0"}).ResolvedChunkSize(); got != DefaultChunkSize {
		t.Fatalf("默认分片=%d, want %d", got, DefaultChunkSize)
	}
}

// testHandler 可注入回调的测试 Handler
type testHandler struct {
	NopHandler
	onOpen    func(Conn)
	onMessage func(Conn, Message)
	onClose   func(Conn, error)
}

// OnOpen 转发到注入回调
func (h *testHandler) OnOpen(c Conn) {
	if h.onOpen != nil {
		h.onOpen(c)
	}
}

// OnMessage 转发到注入回调；未注入时关闭消息
func (h *testHandler) OnMessage(c Conn, msg Message) {
	if h.onMessage != nil {
		h.onMessage(c, msg)
		return
	}
	_ = msg.Close()
}

// OnClose 转发到注入回调
func (h *testHandler) OnClose(c Conn, err error) {
	if h.onClose != nil {
		h.onClose(c, err)
	}
}
