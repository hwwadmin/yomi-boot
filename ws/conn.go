// Package ws 定义 Conn 门面与底层逃生舱
//
// @author zwei
package ws

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/lxzan/gws"
)

// Conn 是 Server 与 Client 共用的连接门面
type Conn interface {
	// WriteText 发送文本消息
	WriteText(ctx context.Context, data []byte) error
	// WriteBinary 发送二进制消息
	WriteBinary(ctx context.Context, data []byte) error
	// WriteReader 从 r 按分片读取并发送；默认一片一完整消息，峰值内存约等于 chunkSize
	WriteReader(ctx context.Context, opcode Opcode, r io.Reader, opts ...WriteOption) error
	// WritePing 发送 Ping
	WritePing(payload []byte) error
	// WritePong 发送 Pong
	WritePong(payload []byte) error
	// Close 发送关闭帧并断开
	Close(code CloseCode, reason string) error
	// Serve 阻塞运行读循环（ReadLoop）；同一连接仅应调用一次，通常 go Serve()
	Serve()
	// RemoteAddr 返回对端地址
	RemoteAddr() net.Addr
	// LocalAddr 返回本地地址
	LocalAddr() net.Addr
	// Session 返回连接级会话存储
	Session() Session
	// SetDeadline 同时设置读写截止时间
	SetDeadline(t time.Time) error
	// SetReadDeadline 设置读截止时间
	SetReadDeadline(t time.Time) error
	// SetWriteDeadline 设置写截止时间
	SetWriteDeadline(t time.Time) error
}

// NativeConn 暴露底层 gws 连接；门面未覆盖的能力经此访问
type NativeConn interface {
	// Native 返回底层 *gws.Conn
	Native() *gws.Conn
}

// connImpl 是 Conn 的 gws 适配实现
type connImpl struct {
	raw       *gws.Conn // raw 是底层连接
	chunkSize int       // chunkSize 是默认分片大小
	serveOnce sync.Once // serveOnce 保证 ReadLoop 只跑一次
}

var (
	_ Conn       = (*connImpl)(nil)
	_ NativeConn = (*connImpl)(nil)
)

// newConn 包装底层连接并写入 Session
func newConn(raw *gws.Conn, chunkSize int) *connImpl {
	c := &connImpl{raw: raw, chunkSize: resolveChunkSize(chunkSize)}
	if raw != nil && raw.Session() != nil {
		raw.Session().Store(connSessionKey, c)
	}
	return c
}

// connFromGWS 从 gws 连接取回门面；若不存在则新建包装
func connFromGWS(raw *gws.Conn, chunkSize int) *connImpl {
	if raw == nil {
		return nil
	}
	if v, ok := raw.Session().Load(connSessionKey); ok {
		if c, ok := v.(*connImpl); ok && c != nil {
			return c
		}
	}
	return newConn(raw, chunkSize)
}

// Native 返回底层 *gws.Conn
func (c *connImpl) Native() *gws.Conn {
	if c == nil {
		return nil
	}
	return c.raw
}

// WriteText 发送文本消息
func (c *connImpl) WriteText(ctx context.Context, data []byte) error {
	return c.writeMessage(ctx, OpcodeText, data)
}

// WriteBinary 发送二进制消息
func (c *connImpl) WriteBinary(ctx context.Context, data []byte) error {
	return c.writeMessage(ctx, OpcodeBinary, data)
}

// writeMessage 在 ctx 未取消时写入一条完整消息
func (c *connImpl) writeMessage(ctx context.Context, opcode Opcode, data []byte) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrClosed)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.raw.WriteMessage(opcode, data); err != nil {
		return errors.Wrap(err, "ws 写入消息失败")
	}
	return nil
}

// WritePing 发送 Ping
func (c *connImpl) WritePing(payload []byte) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrClosed)
	}
	if err := c.raw.WritePing(payload); err != nil {
		return errors.Wrap(err, "ws 写入 Ping 失败")
	}
	return nil
}

// WritePong 发送 Pong
func (c *connImpl) WritePong(payload []byte) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrClosed)
	}
	if err := c.raw.WritePong(payload); err != nil {
		return errors.Wrap(err, "ws 写入 Pong 失败")
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
	if err := c.raw.WriteClose(uint16(code), []byte(reason)); err != nil {
		if errors.Is(err, gws.ErrConnClosed) {
			return nil
		}
		return errors.Wrap(err, "ws 关闭连接失败")
	}
	return nil
}

// Serve 阻塞运行读循环
func (c *connImpl) Serve() {
	if c == nil || c.raw == nil {
		return
	}
	c.serveOnce.Do(func() {
		c.raw.ReadLoop()
	})
}

// RemoteAddr 返回对端地址
func (c *connImpl) RemoteAddr() net.Addr {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.RemoteAddr()
}

// LocalAddr 返回本地地址
func (c *connImpl) LocalAddr() net.Addr {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.LocalAddr()
}

// Session 返回连接级会话存储
func (c *connImpl) Session() Session {
	if c == nil || c.raw == nil {
		return nil
	}
	return sessionFromGWS(c.raw.Session())
}

// SetDeadline 同时设置读写截止时间
func (c *connImpl) SetDeadline(t time.Time) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrClosed)
	}
	return c.raw.SetDeadline(t)
}

// SetReadDeadline 设置读截止时间
func (c *connImpl) SetReadDeadline(t time.Time) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrClosed)
	}
	return c.raw.SetReadDeadline(t)
}

// SetWriteDeadline 设置写截止时间
func (c *connImpl) SetWriteDeadline(t time.Time) error {
	if c == nil || c.raw == nil {
		return errors.Sentinel(ErrClosed)
	}
	return c.raw.SetWriteDeadline(t)
}
