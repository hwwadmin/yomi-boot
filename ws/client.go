// Package ws 实现出站 WebSocket Client（单连接，无 ConnID）
//
// @author zwei
package ws

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Client 是出站客户端：一个实例至多一条连接
type Client interface {
	// Connect 拨号并启动读循环；若已有连接则先关闭再重连
	Connect(ctx context.Context, url string) error
	// Close 关闭当前连接
	Close(code CloseCode, reason string) error
	// Connected 是否持有连接
	Connected() bool

	// Send 按类型发送
	Send(ctx context.Context, typ MsgType, data []byte) error
}

// client 是 Client 的默认实现
type client struct {
	cfg       ClientConfig
	handler   ClientHandler
	heartbeat heartbeatSettings
	writeMax  int
	readMax   int

	mu   sync.Mutex
	conn *connImpl
}

// NewClient 创建出站客户端
func NewClient(cfg ClientConfig) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	interval, timeout := cfg.ResolvedHeartbeat()
	return &client{
		cfg:       cfg,
		handler:   cfg.resolveHandler(),
		heartbeat: heartbeatFromResolved(interval, timeout),
		writeMax:  cfg.ResolvedWriteMaxPayloadSize(),
		readMax:   cfg.ResolvedReadMaxPayloadSize(),
	}, nil
}

// Connect 建立连接并启动读循环
func (c *client) Connect(ctx context.Context, rawURL string) error {
	if c == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if rawURL == "" {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "Connect url 不能为空")
	}

	c.mu.Lock()
	old := c.conn
	c.conn = nil
	c.mu.Unlock()
	if old != nil {
		_ = old.Close(CloseNormalClosure, "reconnect")
	}

	conn, err := c.dial(ctx, rawURL)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	go conn.serve()
	return nil
}

// dial 完成握手并回调 OnConnect；读循环由 Connect 在登记连接后启动
func (c *client) dial(ctx context.Context, rawURL string) (*connImpl, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	handshakeTimeout := effectiveHandshakeTimeout(ctx, c.cfg.ResolvedHandshakeTimeout())
	dialCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	ref := &connRef{}
	raw, _, err := dialRaw(dialCtx, rawURL, dialOptions(c.httpClient(handshakeTimeout), c.cfg.RequestHeader, c.cfg.CompressEnabled, ref))
	if err != nil {
		return nil, errors.Wrap(err, "ws Dial 失败")
	}

	u, _ := url.Parse(rawURL)
	host := ""
	if u != nil {
		host = u.Host
	}
	conn := newConn(connParams{
		raw:      raw,
		remote:   parseNetAddr(host),
		writeMax: c.writeMax,
		readMax:  c.readMax,
	})
	ref.bind(conn)
	conn.onPayload = func(cc *connImpl, p Payload) {
		c.handler.OnPayload(&peerContext{ctx: context.Background(), conn: cc}, p)
	}
	conn.onClose = func(cc *connImpl, closeErr error) {
		c.detach(cc)
		c.handler.OnDisconnect(closeErr)
	}

	c.handler.OnConnect(&peerContext{ctx: context.Background(), conn: conn})
	conn.startHeartbeat(c.heartbeat)
	return conn, nil
}

// httpClient 返回握手用 HTTP 客户端；自定义 HTTPClient 优先
func (c *client) httpClient(handshakeTimeout time.Duration) *http.Client {
	if c.cfg.HTTPClient != nil {
		return c.cfg.HTTPClient
	}
	transport := &http.Transport{
		Proxy:       http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{Timeout: handshakeTimeout}).DialContext,
	}
	if c.cfg.TLSConfig != nil {
		transport.TLSClientConfig = c.cfg.TLSConfig.Clone()
	}
	return &http.Client{Transport: transport}
}

// Close 关闭当前连接
func (c *client) Close(code CloseCode, reason string) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close(code, reason)
}

// Connected 是否已连接
func (c *client) Connected() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

// Send 按类型发送
func (c *client) Send(ctx context.Context, typ MsgType, data []byte) error {
	conn, err := c.mustConn()
	if err != nil {
		return err
	}
	return conn.write(ctx, typ, data)
}

// detach 在对端断开时按连接身份清空当前引用
func (c *client) detach(conn *connImpl) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		c.conn = nil
	}
}

// mustConn 取当前连接；未连接时返回 ErrNotConnected
func (c *client) mustConn() (*connImpl, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil, errors.Sentinel(ErrNotConnected)
	}
	return c.conn, nil
}
