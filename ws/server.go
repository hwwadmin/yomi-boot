// Package ws 实现高级入站 Server 门面
//
// @author zwei
package ws

import (
	"context"
	"io"
	"net"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/ws/wsbase"

	"github.com/lxzan/gws"
)

// Server 是高级入站服务门面：内置 Hub，按框架 ConnID 推送
type Server interface {
	// Start 非阻塞启动
	Start(ctx context.Context) error
	// Close 关闭所有会话并停止监听
	Close(ctx context.Context) error

	// SendText 向指定 ConnID 发送文本
	SendText(connID string, data []byte) error
	// SendBinary 向指定 ConnID 发送二进制
	SendBinary(connID string, data []byte) error
	// SendReader 向指定 ConnID 分片发送
	SendReader(connID string, opcode wsbase.Opcode, r io.Reader, opts ...wsbase.WriteOption) error
	// BroadcastText 广播文本
	BroadcastText(data []byte) error
	// BroadcastBinary 广播二进制
	BroadcastBinary(data []byte) error

	// Len 返回在线会话数
	Len() int
	// Has 判断 ConnID 是否在线
	Has(connID string) bool

	// Hub 返回内置连接表（逃生）
	Hub() *wsbase.Hub
	// GetConn 按 ConnID 取底层连接（逃生）
	GetConn(connID string) (wsbase.Conn, bool)
	// Underlying 返回传输层 Server（逃生）
	Underlying() *wsbase.Server
	// NativeUpgrader 返回底层 Upgrader（逃生）
	NativeUpgrader() *gws.Upgrader
	// ListenAddr 返回实际监听地址
	ListenAddr() (net.Addr, error)
}

// server 是 Server 的默认实现
type server struct {
	raw    *wsbase.Server
	hub    *wsbase.Hub
	bridge *serverBridge
}

// NewServer 创建高级入站服务；ConnID 由框架在 OnOpen 自动生成
func NewServer(cfg ServerConfig) (Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	hub := wsbase.NewHub()
	bridge := newServerBridge(cfg.resolveHandler(), hub)
	raw, err := wsbase.NewServer(bridge, cfg.ServerConfig)
	if err != nil {
		return nil, err
	}
	return &server{raw: raw, hub: hub, bridge: bridge}, nil
}

// Start 启动监听
func (s *server) Start(ctx context.Context) error {
	if s == nil || s.raw == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return s.raw.Start(ctx)
}

// Close 关闭全部会话并停听
func (s *server) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.hub != nil {
		s.hub.CloseAll(wsbase.CloseGoingAway, "server closing")
	}
	if s.raw != nil {
		return s.raw.Close(ctx)
	}
	return nil
}

// SendText 向指定 ConnID 发送文本
func (s *server) SendText(connID string, data []byte) error {
	conn, err := s.mustConn(connID)
	if err != nil {
		return err
	}
	return conn.WriteText(context.Background(), data)
}

// SendBinary 向指定 ConnID 发送二进制
func (s *server) SendBinary(connID string, data []byte) error {
	conn, err := s.mustConn(connID)
	if err != nil {
		return err
	}
	return conn.WriteBinary(context.Background(), data)
}

// SendReader 向指定 ConnID 分片发送
func (s *server) SendReader(connID string, opcode wsbase.Opcode, r io.Reader, opts ...wsbase.WriteOption) error {
	conn, err := s.mustConn(connID)
	if err != nil {
		return err
	}
	return conn.WriteReader(context.Background(), opcode, r, opts...)
}

// BroadcastText 广播文本
func (s *server) BroadcastText(data []byte) error {
	if s == nil || s.hub == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return s.hub.BroadcastText(data)
}

// BroadcastBinary 广播二进制
func (s *server) BroadcastBinary(data []byte) error {
	if s == nil || s.hub == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return s.hub.BroadcastBinary(data)
}

// Len 返回在线数
func (s *server) Len() int {
	if s == nil || s.hub == nil {
		return 0
	}
	return s.hub.Len()
}

// Has 判断是否在线
func (s *server) Has(connID string) bool {
	_, ok := s.GetConn(connID)
	return ok
}

// Hub 返回内置 Hub
func (s *server) Hub() *wsbase.Hub {
	if s == nil {
		return nil
	}
	return s.hub
}

// GetConn 按 ConnID 取连接
func (s *server) GetConn(connID string) (wsbase.Conn, bool) {
	if s == nil || s.hub == nil {
		return nil, false
	}
	return s.hub.Get(connID)
}

// Underlying 返回传输层 Server
func (s *server) Underlying() *wsbase.Server {
	if s == nil {
		return nil
	}
	return s.raw
}

// NativeUpgrader 返回底层 Upgrader
func (s *server) NativeUpgrader() *gws.Upgrader {
	if s == nil || s.raw == nil {
		return nil
	}
	return s.raw.NativeUpgrader()
}

// ListenAddr 返回实际监听地址
func (s *server) ListenAddr() (net.Addr, error) {
	if s == nil || s.raw == nil {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	return s.raw.ListenAddr()
}

func (s *server) mustConn(connID string) (wsbase.Conn, error) {
	if s == nil {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	if connID == "" {
		return nil, errors.Wrap(errors.Sentinel(ErrInvalidArgument), "connID 不能为空")
	}
	conn, ok := s.GetConn(connID)
	if !ok || conn == nil {
		return nil, errors.Sentinel(ErrUnknownPeer)
	}
	return conn, nil
}
