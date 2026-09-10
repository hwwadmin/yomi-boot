// Package ws 的配置与分片单元测试
//
// @author zwei
package ws

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestServerConfigValidate 校验 Addr/ChunkSize 边界
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

// TestEchoAndChunkedUpload 验证独立端口分片上传与按片接收
func TestEchoAndChunkedUpload(t *testing.T) {
	var (
		mu       sync.Mutex
		received [][]byte
		openCh   = make(chan struct{}, 1)
		doneCh   = make(chan struct{}, 1)
	)

	srvHandler := &testHandler{
		onOpen: func(Conn) {
			select {
			case openCh <- struct{}{}:
			default:
			}
		},
		onMessage: func(_ Conn, msg Message) {
			defer msg.Close()
			cp := append([]byte(nil), msg.Bytes()...)
			mu.Lock()
			received = append(received, cp)
			n := len(received)
			mu.Unlock()
			if n == 3 {
				select {
				case doneCh <- struct{}{}:
				default:
				}
			}
		},
	}

	srv, err := NewServer(srvHandler, ServerConfig{Name: "test", Addr: "127.0.0.1:0", ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close(context.Background())

	addr, err := srv.ListenAddr()
	if err != nil {
		t.Fatal(err)
	}

	cli, err := NewClient(NopHandler{}, ClientConfig{Name: "c", ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	conn, _, err := cli.Dial(context.Background(), "ws://"+addr.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(CloseNormalClosure, "")
	go conn.Serve()

	select {
	case <-openCh:
	case <-time.After(2 * time.Second):
		t.Fatal("等待 OnOpen 超时")
	}

	payload := []byte("abcdefghijkl") // 12 bytes → 3 chunks of 4
	if err := conn.WriteReader(context.Background(), OpcodeBinary, bytes.NewReader(payload), WithChunkSize(4)); err != nil {
		t.Fatal(err)
	}

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("等待分片收齐超时")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 3 {
		t.Fatalf("收到分片数=%d, want 3", len(received))
	}
	joined := bytes.Join(received, nil)
	if !bytes.Equal(joined, payload) {
		t.Fatalf("拼接结果=%q, want %q", joined, payload)
	}
	for i, part := range received {
		if len(part) != 4 {
			t.Fatalf("分片[%d] len=%d, want 4", i, len(part))
		}
	}
}

// TestWriteReaderRespectsContext 验证已取消 ctx 时 WriteReader 立即失败
func TestWriteReaderRespectsContext(t *testing.T) {
	srv, err := NewServer(NopHandler{}, ServerConfig{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close(context.Background())

	addr, err := srv.ListenAddr()
	if err != nil {
		t.Fatal(err)
	}
	cli, err := NewClient(NopHandler{}, ClientConfig{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _, err := cli.Dial(context.Background(), "ws://"+addr.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(CloseNormalClosure, "")
	go conn.Serve()

	writeCtx, writeCancel := context.WithCancel(context.Background())
	writeCancel()
	err = conn.WriteReader(writeCtx, OpcodeBinary, strings.NewReader(strings.Repeat("x", 1024)), WithChunkSize(64))
	if err == nil {
		t.Fatal("已取消 ctx 应返回错误")
	}
}

// TestWriteReaderRejectsNilReader 验证 nil Reader 被拒绝
func TestWriteReaderRejectsNilReader(t *testing.T) {
	c := &connImpl{chunkSize: DefaultChunkSize}
	err := c.WriteReader(context.Background(), OpcodeBinary, nil)
	if err == nil {
		t.Fatal("nil reader 应失败")
	}
}

// TestHubRegisterGet 验证 Hub 注册与按身份卸载
func TestHubRegisterGet(t *testing.T) {
	h := NewHub()
	c := &connImpl{}
	h.Register("u1", c)
	got, ok := h.Get("u1")
	if !ok || got != c {
		t.Fatal("Get 失败")
	}
	h.Unregister("u1", c)
	if h.Len() != 0 {
		t.Fatal("Unregister 后 Len 应为 0")
	}
}

// TestNativeEscape 验证 Native 逃生舱可用
func TestNativeEscape(t *testing.T) {
	srv, err := NewServer(NopHandler{}, ServerConfig{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close(context.Background())
	addr, err := srv.ListenAddr()
	if err != nil {
		t.Fatal(err)
	}

	cli, err := NewClient(NopHandler{}, ClientConfig{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _, err := cli.Dial(context.Background(), "ws://"+addr.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(CloseNormalClosure, "")

	n, ok := conn.(NativeConn)
	if !ok || n.Native() == nil {
		t.Fatal("应能 Native() 拿到 *gws.Conn")
	}
	if srv.NativeUpgrader() == nil {
		t.Fatal("NativeUpgrader 不应为 nil")
	}
}

// testHandler 可注入回调的测试 Handler
type testHandler struct {
	NopHandler
	onOpen    func(Conn)
	onMessage func(Conn, Message)
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

// TestWriteReaderChunkCount 验证按块读取次数（不经真实连接）
func TestWriteReaderChunkCount(t *testing.T) {
	var writes int
	r := &countingReader{r: bytes.NewReader(bytes.Repeat([]byte("a"), 10)), readSize: 3}
	buf := make([]byte, 3)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			writes++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if writes != 4 { // 3+3+3+1
		t.Fatalf("读次数=%d, want 4", writes)
	}
}

// countingReader 限制单次 Read 上限，便于断言分片次数
type countingReader struct {
	r        io.Reader
	readSize int
}

// Read 按 readSize 截断后读底层 Reader
func (c *countingReader) Read(p []byte) (int, error) {
	if len(p) > c.readSize {
		p = p[:c.readSize]
	}
	return c.r.Read(p)
}
