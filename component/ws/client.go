// Package ws 实现出站 WebSocket Client（单连接，无 ConnID）
//
// @author zwei
package ws

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// Client 是出站客户端：一实例至多一条连接
type Client interface {
	// Connect 拨号并启动读循环；已有连接则先关旧再重连（未写出队列丢弃）
	Connect(ctx context.Context, url string) error
	// Close 关闭连接并停止队列巡检
	Close(code CloseCode, reason string) error
	// Connected 是否已连接
	Connected() bool
	// Send 入队发送；成功表示已入队；未连接返回 ErrNotConnected
	Send(typ MsgType, data []byte) error
}

// client 是 Client 的默认实现
type client struct {
	cfg             ClientConfig
	handler         ClientHandler
	heartbeat       heartbeatSettings
	writeMax        int
	readMax         int
	maxFrame        int
	sendQueueSize   int
	queueGCInterval time.Duration

	mu        sync.Mutex // mu 保护 conn 与 gc 状态
	connectMu sync.Mutex // connectMu 串行化 Connect，避免并发拨号孤儿连接
	conn      *connImpl
	gcStop    chan struct{} // gcStop 非 nil 且未关闭时巡检运行中
	gcStarted bool
}

// NewClient 创建出站客户端
func NewClient(cfg ClientConfig) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	interval, timeout := cfg.ResolvedHeartbeat()
	return &client{
		cfg:             cfg,
		handler:         cfg.resolveHandler(),
		heartbeat:       heartbeatFromResolved(interval, timeout),
		writeMax:        cfg.ResolvedWriteMaxPayloadSize(),
		readMax:         cfg.ResolvedReadMaxPayloadSize(),
		maxFrame:        cfg.MaxFramePayloadSize,
		sendQueueSize:   cfg.SendQueueSize,
		queueGCInterval: cfg.ResolvedQueueGCInterval(),
	}, nil
}

// Connect 建立连接并启动读循环（同一 Client 串行）
func (c *client) Connect(ctx context.Context, rawURL string) error {
	if c == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if rawURL == "" {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "Connect url 不能为空")
	}

	c.connectMu.Lock()
	defer c.connectMu.Unlock()

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
	c.ensureQueueGC()
	go conn.serve()
	return nil
}

// dial 握手并 OnConnect；失败或 panic 时关闭连接
func (c *client) dial(ctx context.Context, rawURL string) (out *connImpl, err error) {
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
		raw:           raw,
		remote:        parseNetAddr(host),
		writeMax:      c.writeMax,
		readMax:       c.readMax,
		maxFrame:      c.maxFrame,
		sendQueueSize: c.sendQueueSize,
	})
	ref.bind(conn)
	conn.onPayload = func(cc *connImpl, p Payload) {
		c.handler.OnPayload(&peerContext{ctx: context.Background(), conn: cc}, p)
	}
	conn.onClose = func(cc *connImpl, closeErr error) {
		c.detach(cc)
		c.handler.OnDisconnect(closeErr)
	}

	defer func() {
		if rec := recover(); rec != nil {
			_ = conn.Close(CloseGoingAway, "on_connect panic")
			out = nil
			err = errors.Wrap(fmt.Errorf("%v", rec), "ws OnConnect panic")
		}
	}()

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

// Close 关闭当前连接并停止队列巡检
func (c *client) Close(code CloseCode, reason string) error {
	if c == nil {
		return nil
	}
	c.stopQueueGC()
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

// Send 入队发送
func (c *client) Send(typ MsgType, data []byte) error {
	conn, err := c.mustConn()
	if err != nil {
		return err
	}
	return conn.enqueue(typ, data)
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

// ensureQueueGC 启动队列巡检（可在 Close 后再启）
func (c *client) ensureQueueGC() {
	if c == nil || c.queueGCInterval <= 0 {
		return
	}
	c.mu.Lock()
	if c.gcStarted {
		c.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	c.gcStop = stopCh
	c.gcStarted = true
	c.mu.Unlock()
	go func() {
		ticker := time.NewTicker(c.queueGCInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				c.purgeDeadQueue()
			}
		}
	}()
}

// stopQueueGC 停止巡检（幂等）
func (c *client) stopQueueGC() {
	if c == nil {
		return
	}
	c.mu.Lock()
	stopCh := c.gcStop
	c.gcStop = nil
	c.gcStarted = false
	c.mu.Unlock()
	if stopCh != nil {
		close(stopCh)
	}
}

// purgeDeadQueue 清理当前连接上已关闭的发送队列
func (c *client) purgeDeadQueue() {
	if c == nil {
		return
	}
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil || conn.sendQ == nil {
		return
	}
	if conn.sendQ.isClosed() {
		conn.sendQ.purgeIfDead()
	}
}
