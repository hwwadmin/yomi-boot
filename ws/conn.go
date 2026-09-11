// Package ws 包装 coder/websocket 连接：读写、读循环、心跳与寿命挂在连接字段上
//
// @author zwei
package ws

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/coder/websocket"
)

// connImpl 是一条 WebSocket 连接的内部实现
type connImpl struct {
	raw      *websocket.Conn // raw 是底层连接
	session  *session        // session 是业务可用的连接级存储
	remote   net.Addr        // remote 是对端地址
	local    net.Addr        // local 是本地地址
	writeMax int             // writeMax 是单条可写上限
	connID   string          // connID 仅服务端在登记后赋值

	heartbeat     *heartbeatRunner // heartbeat 是本连接心跳；无心跳时为 nil
	lifetimeTimer *time.Timer      // lifetimeTimer 是通道最长存活定时器

	onPayload func(*connImpl, Payload) // onPayload 收到业务消息
	onClose   func(*connImpl, error)   // onClose 读循环结束

	serveOnce   sync.Once          // serveOnce 保证读循环只跑一次
	serveCtx    context.Context    // serveCtx 在 Close 时取消以唤醒 Read
	serveCancel context.CancelFunc // serveCancel 取消读循环
}

// connRef 在 Accept/Dial 回调里绑定已创建的连接，供 Ping/Pong 续命
type connRef struct {
	p atomic.Pointer[connImpl] // p 指向已绑定的连接
}

// bind 绑定连接
func (r *connRef) bind(c *connImpl) {
	if r == nil {
		return
	}
	r.p.Store(c)
}

// onPing 入站 Ping 时刷新空闲超时并允许自动回 Pong
func (r *connRef) onPing(_ context.Context, _ []byte) bool {
	if r == nil {
		return true
	}
	if c := r.p.Load(); c != nil {
		c.touchHeartbeat()
	}
	return true
}

// onPong 入站 Pong 时刷新空闲超时
func (r *connRef) onPong(_ context.Context, _ []byte) {
	if r == nil {
		return
	}
	if c := r.p.Load(); c != nil {
		c.touchHeartbeat()
	}
}

// connParams 是 newConn 的构造参数
type connParams struct {
	raw      *websocket.Conn
	remote   net.Addr
	local    net.Addr
	writeMax int
	readMax  int
}

// newConn 包装底层连接
func newConn(p connParams) *connImpl {
	if p.writeMax <= 0 {
		p.writeMax = DefaultWriteMaxPayloadSize
	}
	if p.raw != nil && p.readMax > 0 {
		p.raw.SetReadLimit(int64(p.readMax))
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &connImpl{
		raw:         p.raw,
		session:     newSession(),
		remote:      p.remote,
		local:       p.local,
		writeMax:    p.writeMax,
		serveCtx:    ctx,
		serveCancel: cancel,
	}
}

// write 按业务类型写入一条消息
func (c *connImpl) write(ctx context.Context, typ MsgType, data []byte) error {
	switch typ {
	case MsgText:
		return c.writeMessage(ctx, websocket.MessageText, data)
	case MsgBinary:
		return c.writeMessage(ctx, websocket.MessageBinary, data)
	default:
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "不支持的 MsgType")
	}
}

// writeMessage 在 ctx 未取消时写入一条完整消息
func (c *connImpl) writeMessage(ctx context.Context, typ websocket.MessageType, data []byte) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrClosed)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) > c.writeMax {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "超过 WriteMaxPayloadSize")
	}
	if err := c.raw.Write(ctx, typ, data); err != nil {
		return errors.Wrap(err, "ws 写入消息失败")
	}
	return nil
}

// ping 发送 Ping 并等待 Pong
func (c *connImpl) ping(ctx context.Context) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrClosed)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.raw.Ping(ctx); err != nil {
		return errors.Wrap(err, "ws Ping 失败")
	}
	return nil
}

// Close 发送关闭帧并断开
func (c *connImpl) Close(code CloseCode, reason string) error {
	if c == nil || c.raw == nil {
		return nil
	}
	if code == 0 {
		code = CloseNormalClosure
	}
	err := c.raw.Close(websocket.StatusCode(code), reason)
	if c.serveCancel != nil {
		c.serveCancel()
	}
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return errors.Wrap(err, "ws 关闭连接失败")
	}
	return nil
}

// serve 阻塞运行读循环；同一连接仅应调用一次
func (c *connImpl) serve() {
	if c == nil || c.raw == nil {
		return
	}
	c.serveOnce.Do(func() {
		var err error
		defer func() {
			if rec := recover(); rec != nil {
				err = errors.Wrap(fmt.Errorf("%v", rec), "ws 读循环 panic")
			}
			c.stopHeartbeat()
			c.stopLifetime()
			if c.onClose != nil {
				c.onClose(c, err)
			}
			_ = c.raw.CloseNow()
		}()
		for {
			var typ websocket.MessageType
			var data []byte
			typ, data, err = c.raw.Read(c.serveCtx)
			if err != nil {
				return
			}
			c.touchHeartbeat()
			p, ok := payloadFromWire(typ, data)
			if !ok {
				continue
			}
			if c.onPayload != nil {
				c.onPayload(c, p)
			}
		}
	})
}

// payloadFromWire 将协议消息类型映射为业务 Payload
func payloadFromWire(typ websocket.MessageType, data []byte) (Payload, bool) {
	switch typ {
	case websocket.MessageText:
		return Payload{Type: MsgText, Data: data}, true
	case websocket.MessageBinary:
		return Payload{Type: MsgBinary, Data: data}, true
	default:
		return Payload{}, false
	}
}

// compressionMode 将 CompressEnabled 映射为 coder 压缩模式
func compressionMode(enabled bool) websocket.CompressionMode {
	if enabled {
		return websocket.CompressionContextTakeover
	}
	return websocket.CompressionDisabled
}

// acceptOptions 构造服务端握手选项
func acceptOptions(origin []string, compress bool, ref *connRef) *websocket.AcceptOptions {
	return &websocket.AcceptOptions{
		InsecureSkipVerify: len(origin) == 0,
		OriginPatterns:     origin,
		CompressionMode:    compressionMode(compress),
		OnPingReceived:     ref.onPing,
		OnPongReceived:     ref.onPong,
	}
}

// dialOptions 构造客户端握手选项
func dialOptions(httpClient *http.Client, header http.Header, compress bool, ref *connRef) *websocket.DialOptions {
	return &websocket.DialOptions{
		HTTPClient:      httpClient,
		HTTPHeader:      header,
		CompressionMode: compressionMode(compress),
		OnPingReceived:  ref.onPing,
		OnPongReceived:  ref.onPong,
	}
}

// acceptRaw 完成服务端 WebSocket 升级
func acceptRaw(w http.ResponseWriter, r *http.Request, opts *websocket.AcceptOptions) (*websocket.Conn, error) {
	return websocket.Accept(w, r, opts)
}

// dialRaw 完成客户端 WebSocket 握手
func dialRaw(ctx context.Context, rawURL string, opts *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
	return websocket.Dial(ctx, rawURL, opts)
}

// parseNetAddr 将 host:port 解析为 TCP 地址；失败时退回字符串地址
func parseNetAddr(s string) net.Addr {
	if s == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return stringAddr(s)
	}
	p, convErr := strconv.Atoi(port)
	if convErr != nil {
		return stringAddr(s)
	}
	ip := net.ParseIP(host)
	return &net.TCPAddr{IP: ip, Port: p}
}

// stringAddr 是无法解析时的兜底 net.Addr
type stringAddr string

// Network 返回网络类型
func (a stringAddr) Network() string { return "tcp" }

// String 返回原始地址串
func (a stringAddr) String() string { return string(a) }

// effectiveHandshakeTimeout 取配置握手超时与 ctx 剩余截止时间的较早者
func effectiveHandshakeTimeout(ctx context.Context, base time.Duration) time.Duration {
	if base <= 0 {
		base = DefaultHandshakeTimeout
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return base
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Nanosecond
	}
	if remaining < base {
		return remaining
	}
	return base
}
