// Package wsbase 的 Server/Client 联调测试
//
// @author zwei
package wsbase

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
	wsURL      string

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

	t.Logf("[流程] 准备启动 Server name=%q addr=%q writeMaxPayload=%d", srvCfg.Name, srvCfg.Addr, srvCfg.ResolvedWriteMaxPayloadSize())

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
			t.Logf("[服务端] OnOpen remote=%v local=%v", c.RemoteAddr(), c.LocalAddr())
			select {
			case env.serverOpen <- c:
			default:
			}
		},
		onMessage: func(_ Conn, msg Message) {
			defer func() { _ = msg.Close() }()
			cp := append([]byte(nil), msg.Bytes()...)
			t.Logf("[服务端] OnMessage opcode=%v len=%d payload=%q", msg.Opcode(), len(cp), previewBytes(cp))
			select {
			case env.serverMsgs <- cp:
			default:
				t.Error("serverMsgs 通道已满")
			}
		},
		onClose: func(c Conn, err error) {
			t.Logf("[服务端] OnClose remote=%v err=%v", addrOrNil(c), err)
			select {
			case env.serverClose <- struct{}{}:
			default:
			}
		},
	}
	cliHandler := &testHandler{
		onOpen: func(c Conn) {
			t.Logf("[客户端] OnOpen remote=%v local=%v", c.RemoteAddr(), c.LocalAddr())
			select {
			case env.clientOpen <- c:
			default:
			}
		},
		onMessage: func(_ Conn, msg Message) {
			defer func() { _ = msg.Close() }()
			cp := append([]byte(nil), msg.Bytes()...)
			t.Logf("[客户端] OnMessage opcode=%v len=%d payload=%q", msg.Opcode(), len(cp), previewBytes(cp))
			select {
			case env.clientMsgs <- cp:
			default:
				t.Error("clientMsgs 通道已满")
			}
		},
		onClose: func(c Conn, err error) {
			t.Logf("[客户端] OnClose remote=%v err=%v", addrOrNil(c), err)
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
	env.wsURL = "ws://" + addr.String() + "/"
	t.Logf("[流程] Server 已监听 listenAddr=%v", addr)

	t.Logf("[流程] 准备创建 Client name=%q writeMaxPayload=%d", cliCfg.Name, cliCfg.ResolvedWriteMaxPayloadSize())
	cli, err := NewClient(cliHandler, cliCfg)
	if err != nil {
		env.close()
		t.Fatalf("NewClient: %v", err)
	}
	env.client = cli

	t.Logf("[流程] Client Dial url=%s", env.wsURL)
	conn, _, err := cli.Dial(context.Background(), env.wsURL)
	if err != nil {
		env.close()
		t.Fatalf("Client.Dial: %v", err)
	}
	env.clientConn = conn
	go conn.Serve()
	t.Logf("[流程] Client Serve 已启动")

	select {
	case env.serverConn = <-env.serverOpen:
		t.Logf("[流程] 已等到服务端 OnOpen")
	case <-time.After(testWait):
		env.close()
		t.Fatal("等待服务端 OnOpen 超时")
	}
	select {
	case <-env.clientOpen:
		t.Logf("[流程] 已等到客户端 OnOpen，握手完成")
	case <-time.After(testWait):
		env.close()
		t.Fatal("等待客户端 OnOpen 超时")
	}
	return env
}

// close 关闭客户端连接与 Server
func (env *testEnv) close() {
	env.t.Logf("[流程] 开始关闭：先关 Client 连接，再停 Server")
	if env.clientConn != nil {
		_ = env.clientConn.Close(CloseNormalClosure, "test done")
	}
	if env.cancel != nil {
		env.cancel()
	}
	if env.srv != nil {
		_ = env.srv.Close(context.Background())
	}
	env.t.Logf("[流程] 关闭完成")
}

// waitServerMsg 等待服务端收到一条消息
func (env *testEnv) waitServerMsg() []byte {
	env.t.Helper()
	select {
	case b := <-env.serverMsgs:
		env.t.Logf("[流程] 断言侧取到服务端消息 len=%d payload=%q", len(b), previewBytes(b))
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
		env.t.Logf("[流程] 断言侧取到客户端消息 len=%d payload=%q", len(b), previewBytes(b))
		return b
	case <-time.After(testWait):
		env.t.Fatal("等待客户端消息超时")
		return nil
	}
}

// previewBytes 截断过长载荷便于日志阅读
func previewBytes(b []byte) string {
	const max = 64
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}

// addrOrNil 安全打印连接远端地址
func addrOrNil(c Conn) any {
	if c == nil {
		return nil
	}
	return c.RemoteAddr()
}

// TestServerAndClientConnect 基本用例：启动 Server，Client 拨号连上，双方 OnOpen
func TestServerAndClientConnect(t *testing.T) {
	t.Log("[用例] Server 启动 + Client 连接")
	env := startServerAndClient(t, ServerConfig{Name: "srv"}, ClientConfig{Name: "cli"})
	defer env.close()

	t.Logf("[断言] Server.Name=%q Client.Name=%q", env.srv.Name(), env.client.Name())
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
	t.Logf("[断言] 连接就绪 clientRemote=%v serverRemote=%v", env.clientConn.RemoteAddr(), env.serverConn.RemoteAddr())
}

// TestServerClientTextEcho 客户端发文本，服务端收到后再回写，客户端收到
func TestServerClientTextEcho(t *testing.T) {
	t.Log("[用例] 文本双向回声")
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	want := []byte("hello-ws")
	t.Logf("[流程] 客户端 WriteText %q", want)
	if err := env.clientConn.WriteText(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got := env.waitServerMsg()
	if !bytes.Equal(got, want) {
		t.Fatalf("服务端收到=%q, want %q", got, want)
	}
	t.Log("[断言] 服务端收到客户端文本，准备回写")

	reply := []byte("pong-text")
	t.Logf("[流程] 服务端 WriteText %q", reply)
	if err := env.serverConn.WriteText(context.Background(), reply); err != nil {
		t.Fatal(err)
	}
	got = env.waitClientMsg()
	if !bytes.Equal(got, reply) {
		t.Fatalf("客户端收到=%q, want %q", got, reply)
	}
	t.Log("[断言] 客户端收到服务端回写，用例通过")
}

// TestServerClientBinaryEcho 客户端发二进制，服务端收到后再回写
func TestServerClientBinaryEcho(t *testing.T) {
	t.Log("[用例] 二进制双向回声")
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	want := []byte{0x00, 0x01, 0xff, 0xfe}
	t.Logf("[流程] 客户端 WriteBinary len=%d", len(want))
	if err := env.clientConn.WriteBinary(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got := env.waitServerMsg()
	if !bytes.Equal(got, want) {
		t.Fatalf("服务端收到=%v, want %v", got, want)
	}
	t.Log("[断言] 服务端收到二进制，准备回写")

	t.Logf("[流程] 服务端 WriteBinary len=%d", len(want))
	if err := env.serverConn.WriteBinary(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got = env.waitClientMsg()
	if !bytes.Equal(got, want) {
		t.Fatalf("客户端收到=%v, want %v", got, want)
	}
	t.Log("[断言] 客户端收到服务端二进制回写，用例通过")
}

// TestServerClientBinaryRoundTrip 二进制整包往返（一次 WriteBinary → 一次 OnMessage）
func TestServerClientBinaryRoundTrip(t *testing.T) {
	t.Log("[用例] 客户端 WriteBinary 整包上传")
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	payload := []byte("abcdefghijkl")
	t.Logf("[流程] 客户端 WriteBinary 总长=%d", len(payload))
	if err := env.clientConn.WriteBinary(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	got := env.waitServerMsg()
	if !bytes.Equal(got, payload) {
		t.Fatalf("服务端收到=%q want %q", got, payload)
	}
	t.Log("[断言] 服务端一次收到完整载荷")
}

// TestServerClientBinaryDownload 服务端整包下发
func TestServerClientBinaryDownload(t *testing.T) {
	t.Log("[用例] 服务端 WriteBinary 整包下发")
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	payload := []byte("0123456789")
	t.Logf("[流程] 服务端 WriteBinary 总长=%d", len(payload))
	if err := env.serverConn.WriteBinary(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	got := env.waitClientMsg()
	if !bytes.Equal(got, payload) {
		t.Fatalf("客户端收到=%q want %q", got, payload)
	}
	t.Log("[断言] 客户端一次收到完整载荷")
}

// TestServerClientWriteBinaryCanceledContext 已取消 ctx 时写入失败
func TestServerClientWriteBinaryCanceledContext(t *testing.T) {
	t.Log("[用例] 已取消 ctx 的 WriteBinary 应失败")
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	writeCtx, writeCancel := context.WithCancel(context.Background())
	writeCancel()
	t.Log("[流程] 使用已取消的 ctx 调用 WriteBinary")
	err := env.clientConn.WriteBinary(writeCtx, []byte(strings.Repeat("x", 64)))
	if err == nil {
		t.Fatal("已取消 ctx 应返回错误")
	}
	t.Logf("[断言] 得到预期错误: %v", err)
}

// TestServerClientNative 验证连上后 Native 逃生舱可用
func TestServerClientNative(t *testing.T) {
	t.Log("[用例] Native 逃生舱")
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	n, ok := env.clientConn.(NativeConn)
	if !ok || n.Native() == nil {
		t.Fatal("客户端 Conn 应支持 Native()")
	}
	t.Logf("[断言] 客户端 Native()=%T 非空", n.Native())
	n2, ok := env.serverConn.(NativeConn)
	if !ok || n2.Native() == nil {
		t.Fatal("服务端 Conn 应支持 Native()")
	}
	t.Logf("[断言] 服务端 Native()=%T 非空", n2.Native())
	if env.srv.NativeUpgrader() == nil {
		t.Fatal("NativeUpgrader 不应为 nil")
	}
	t.Log("[断言] NativeUpgrader 非空，用例通过")
}

// TestServerClientHub 连上后将服务端连接注册进 Hub 并可取出
func TestServerClientHub(t *testing.T) {
	t.Log("[用例] Hub 注册/查询/卸载")
	env := startServerAndClient(t, ServerConfig{}, ClientConfig{})
	defer env.close()

	hub := NewHub()
	t.Log("[流程] Hub.Register id=u1")
	hub.Register("u1", env.serverConn)
	got, ok := hub.Get("u1")
	if !ok || got != env.serverConn {
		t.Fatal("Hub.Get 失败")
	}
	t.Logf("[断言] Hub.Get 成功 Len=%d", hub.Len())
	if hub.Len() != 1 {
		t.Fatalf("Len=%d", hub.Len())
	}
	t.Log("[流程] Hub.Unregister id=u1")
	hub.Unregister("u1", env.serverConn)
	if hub.Len() != 0 {
		t.Fatal("Unregister 后应为空")
	}
	t.Log("[断言] Hub 已清空，用例通过")
}

// TestServerConfigValidate 校验 Addr / WriteMaxPayloadSize 边界（无网络）
func TestServerConfigValidate(t *testing.T) {
	t.Log("[用例] ServerConfig.Validate 边界")
	if err := (ServerConfig{}).Validate(); err == nil {
		t.Fatal("空 Addr 应失败")
	}
	t.Log("[断言] 空 Addr 校验失败，符合预期")
	if err := (ServerConfig{Addr: ":0", WriteMaxPayloadSize: -1}).Validate(); err == nil {
		t.Fatal("负 WriteMaxPayloadSize 应失败")
	}
	t.Log("[断言] 负 WriteMaxPayloadSize 校验失败，符合预期")
	if err := (ServerConfig{Addr: ":0", WriteMaxPayloadSize: MaxWriteMaxPayloadSize + 1}).Validate(); err == nil {
		t.Fatal("超大 WriteMaxPayloadSize 应失败")
	}
	t.Log("[断言] 超大 WriteMaxPayloadSize 校验失败，符合预期")
	if err := (ServerConfig{Addr: ":0"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if got := (ServerConfig{Addr: ":0"}).ResolvedWriteMaxPayloadSize(); got != DefaultWriteMaxPayloadSize {
		t.Fatalf("默认可写上限=%d, want %d", got, DefaultWriteMaxPayloadSize)
	}
	t.Logf("[断言] 合法配置通过，默认 WriteMaxPayloadSize=%d", DefaultWriteMaxPayloadSize)
	if err := (ServerConfig{Addr: ":0", MaxConnections: -1}).Validate(); err == nil {
		t.Fatal("负 MaxConnections 应失败")
	}
	if err := (ServerConfig{Addr: ":0", MaxConnections: 0}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ServerConfig{Addr: ":0", MaxConnections: 10}).Validate(); err != nil {
		t.Fatal(err)
	}
	t.Log("[断言] MaxConnections 0/正数通过，负数失败")
}

// TestMaxConnectionsRejectsExtra 上限为 1 时第二条连接握手后应被断开
func TestMaxConnectionsRejectsExtra(t *testing.T) {
	t.Log("[用例] MaxConnections=1 拒绝超额连接")
	zero := Duration(0)
	env := startServerAndClient(t,
		ServerConfig{
			MaxConnections:        1,
			HeartbeatInterval:     zero,
			HeartbeatTimeout:      zero,
			MaxConnLifetime:       zero,
			MaxConnLifetimeJitter: zero,
		},
		ClientConfig{
			HeartbeatInterval: zero,
			HeartbeatTimeout:  zero,
		},
	)
	defer env.close()

	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		if env.srv.ActiveConns() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if env.srv.ActiveConns() != 1 {
		t.Fatalf("首连 ActiveConns=%d want 1", env.srv.ActiveConns())
	}

	extraOpen := make(chan struct{}, 1)
	extraClose := make(chan struct{}, 1)
	extraHandler := &testHandler{
		onOpen: func(Conn) {
			select {
			case extraOpen <- struct{}{}:
			default:
			}
		},
		onClose: func(Conn, error) {
			select {
			case extraClose <- struct{}{}:
			default:
			}
		},
	}
	cli, err := NewClient(extraHandler, ClientConfig{
		HeartbeatInterval: zero,
		HeartbeatTimeout:  zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	conn, _, err := cli.Dial(context.Background(), env.wsURL)
	if err != nil {
		t.Fatal(err)
	}
	go conn.Serve()

	select {
	case <-extraClose:
		t.Log("[断言] 超额连接已被服务端断开")
	case <-time.After(testWait):
		t.Fatal("等待超额连接关闭超时")
	}
	_ = conn.Close(CloseNormalClosure, "cleanup")

	deadline = time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		if env.srv.ActiveConns() == 1 {
			t.Logf("[断言] 首连仍占槽 ActiveConns=%d", env.srv.ActiveConns())
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("首连应保留槽 ActiveConns=%d", env.srv.ActiveConns())
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
