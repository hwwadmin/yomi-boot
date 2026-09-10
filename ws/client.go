// Package ws 实现高级出站 Client 门面（单连接，无 ConnID 概念）
//
// @author zwei
package ws

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/ws/wsbase"
)

// Client 是高级出站客户端门面：一个实例至多一条连接
type Client interface {
	// Connect 拨号并自动 Serve；若已有连接则先关闭再重连
	Connect(ctx context.Context, url string) error
	// Close 关闭当前连接
	Close(code wsbase.CloseCode, reason string) error
	// Connected 是否持有连接
	Connected() bool

	// Send 按类型发送
	Send(ctx context.Context, typ MsgType, data []byte) error

	// Conn 返回当前底层连接（逃生）；未连接时 ok=false
	Conn() (wsbase.Conn, bool)
	// Underlying 返回传输层 Client（逃生）
	Underlying() *wsbase.Client
}

// client 是 Client 的默认实现
type client struct {
	raw    *wsbase.Client
	bridge *clientBridge

	mu   sync.Mutex
	conn wsbase.Conn
}

// NewClient 创建高级出站客户端
func NewClient(cfg ClientConfig) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cli := &client{}
	bridge := newClientBridge(cfg.resolveHandler())
	bridge.onDetach = cli.detachConn
	raw, err := wsbase.NewClient(bridge, cfg.ClientConfig)
	if err != nil {
		return nil, err
	}
	cli.raw = raw
	cli.bridge = bridge
	return cli, nil
}

// Connect 建立连接并启动读循环
func (c *client) Connect(ctx context.Context, url string) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if url == "" {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "Connect url 不能为空")
	}

	c.mu.Lock()
	old := c.conn
	c.conn = nil
	c.mu.Unlock()
	if old != nil {
		_ = old.Close(wsbase.CloseNormalClosure, "reconnect")
	}

	conn, _, err := c.raw.Dial(ctx, url)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	c.bridge.setConn(conn)
	go conn.Serve()
	return nil
}

// Close 关闭当前连接
func (c *client) Close(code wsbase.CloseCode, reason string) error {
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
	return writeByType(ctx, conn, typ, data)
}

// Conn 返回当前连接
func (c *client) Conn() (wsbase.Conn, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil, false
	}
	return c.conn, true
}

// Underlying 返回传输层 Client
func (c *client) Underlying() *wsbase.Client {
	if c == nil {
		return nil
	}
	return c.raw
}

// detachConn 在对端断开时按连接身份清空当前引用
func (c *client) detachConn(conn wsbase.Conn) {
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
func (c *client) mustConn() (wsbase.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil, errors.Sentinel(ErrNotConnected)
	}
	return c.conn, nil
}
