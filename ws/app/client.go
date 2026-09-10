// Package app 实现高级出站 Client 门面（单连接，无 ConnID 概念）
//
// @author zwei
package app

import (
	"context"
	"io"
	"sync"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/ws"
)

// Client 是高级出站客户端门面：一个实例至多一条连接
type Client interface {
	// Connect 拨号并自动 Serve；若已有连接则先关闭再重连
	Connect(ctx context.Context, url string) error
	// Close 关闭当前连接
	Close(code ws.CloseCode, reason string) error
	// Connected 是否持有连接
	Connected() bool

	// SendText 发送文本
	SendText(ctx context.Context, data []byte) error
	// SendBinary 发送二进制
	SendBinary(ctx context.Context, data []byte) error
	// SendReader 分片发送
	SendReader(ctx context.Context, opcode ws.Opcode, r io.Reader, opts ...ws.WriteOption) error

	// Conn 返回当前底层连接（逃生）；未连接时 ok=false
	Conn() (ws.Conn, bool)
	// Underlying 返回传输层 Client（逃生）
	Underlying() *ws.Client
}

// client 是 Client 的默认实现
type client struct {
	raw    *ws.Client
	bridge *clientBridge

	mu   sync.Mutex
	conn ws.Conn
}

// NewClient 创建高级出站客户端
func NewClient(h ClientHandler, cfg ClientConfig) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	bridge := newClientBridge(h)
	raw, err := ws.NewClient(bridge, cfg.ClientConfig)
	if err != nil {
		return nil, err
	}
	return &client{raw: raw, bridge: bridge}, nil
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
		_ = old.Close(ws.CloseNormalClosure, "reconnect")
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
func (c *client) Close(code ws.CloseCode, reason string) error {
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

// SendText 发送文本
func (c *client) SendText(ctx context.Context, data []byte) error {
	conn, err := c.mustConn()
	if err != nil {
		return err
	}
	return conn.WriteText(ctx, data)
}

// SendBinary 发送二进制
func (c *client) SendBinary(ctx context.Context, data []byte) error {
	conn, err := c.mustConn()
	if err != nil {
		return err
	}
	return conn.WriteBinary(ctx, data)
}

// SendReader 分片发送
func (c *client) SendReader(ctx context.Context, opcode ws.Opcode, r io.Reader, opts ...ws.WriteOption) error {
	conn, err := c.mustConn()
	if err != nil {
		return err
	}
	return conn.WriteReader(ctx, opcode, r, opts...)
}

// Conn 返回当前连接
func (c *client) Conn() (ws.Conn, bool) {
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
func (c *client) Underlying() *ws.Client {
	if c == nil {
		return nil
	}
	return c.raw
}

func (c *client) mustConn() (ws.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil, errors.Sentinel(ErrNotConnected)
	}
	return c.conn, nil
}
