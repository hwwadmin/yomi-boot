// Package wsbase 实现独立端口 WebSocket Server
//
// @author zwei
package wsbase

import (
	"bufio"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/lxzan/gws"
)

// Server 是独立端口的 WebSocket 服务实例；可 New 多个
type Server struct {
	name      string        // name 是实例名
	addr      string        // addr 是配置的监听地址
	upgrader  *gws.Upgrader // upgrader 负责握手升级
	tlsConfig *tls.Config   // tlsConfig 供 StartTLS 合并证书

	listener net.Listener // listener 是当前监听器；关闭后由 serveLoop 持有局部引用
	mu       sync.Mutex   // mu 保护 listener
	closed   atomic.Bool  // closed 表示已关闭
	started  atomic.Bool  // started 表示已启动
}

// NewServer 根据配置创建 Server；调用 Start / StartTLS 后开始接受连接
func NewServer(h Handler, cfg ServerConfig) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	interval, timeout := cfg.ResolvedHeartbeat()
	adapter := newEventAdapter(h, heartbeatFromResolved(interval, timeout))
	upgrader := gws.NewUpgrader(adapter, cfg.toGWSOption())
	return &Server{
		name:      cfg.Name,
		addr:      cfg.Addr,
		upgrader:  upgrader,
		tlsConfig: cfg.TLSConfig,
	}, nil
}

// Name 返回实例名
func (s *Server) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Addr 返回配置的监听地址（非实际 LocalAddr）
func (s *Server) Addr() string {
	if s == nil {
		return ""
	}
	return s.addr
}

// NativeUpgrader 返回底层 gws.Upgrader，供未封装能力使用
func (s *Server) NativeUpgrader() *gws.Upgrader {
	if s == nil {
		return nil
	}
	return s.upgrader
}

// Start 非阻塞启动明文监听；ctx 取消时关闭服务
func (s *Server) Start(ctx context.Context) error {
	return s.start(ctx, false, "", "")
}

// StartTLS 非阻塞启动 TLS 监听；certFile/keyFile 为证书路径
func (s *Server) StartTLS(ctx context.Context, certFile, keyFile string) error {
	if certFile == "" || keyFile == "" {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "StartTLS 需要 certFile 与 keyFile")
	}
	return s.start(ctx, true, certFile, keyFile)
}

// start 监听并启动接受循环；useTLS 为 true 时包装 TLS
func (s *Server) start(ctx context.Context, useTLS bool, certFile, keyFile string) error {
	if s == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if !s.started.CompareAndSwap(false, true) {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "Server 已启动")
	}
	if s.closed.Load() {
		return errors.Sentinel(ErrClosed)
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		s.started.Store(false)
		return errors.Wrap(err, "ws Server 监听失败")
	}
	if useTLS {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			_ = ln.Close()
			s.started.Store(false)
			return errors.Wrap(err, "ws Server 加载证书失败")
		}
		tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"http/1.1"}}
		if s.tlsConfig != nil {
			tlsCfg = s.tlsConfig.Clone()
			tlsCfg.Certificates = []tls.Certificate{cert}
			if len(tlsCfg.NextProtos) == 0 {
				tlsCfg.NextProtos = []string{"http/1.1"}
			}
		}
		ln = tls.NewListener(ln, tlsCfg)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	go s.serveLoop(ln)
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = s.Close(context.Background())
		}()
	}
	return nil
}

// serveLoop 在 ln 上接受连接；须传入局部 listener，避免 Close 置空字段后空指针
func (s *Server) serveLoop(ln net.Listener) {
	for {
		netConn, err := ln.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			continue
		}
		go s.handleConn(netConn)
	}
}

// handleConn 完成握手并阻塞运行该连接的读循环
func (s *Server) handleConn(netConn net.Conn) {
	br := bufio.NewReader(netConn)
	req, err := http.ReadRequest(br)
	if err != nil {
		_ = netConn.Close()
		return
	}
	socket, err := s.upgrader.UpgradeFromConn(netConn, br, req)
	if err != nil {
		_ = netConn.Close()
		return
	}
	// 服务端在本 goroutine 跑读循环；Conn.Serve 因 sync.Once 可再调但不会重复
	conn := connFromGWS(socket)
	conn.Serve()
}

// Close 停止监听；已建立连接需业务侧 Close 或对端断开
func (s *Server) Close(_ context.Context) error {
	if s == nil {
		return nil
	}
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.mu.Lock()
	ln := s.listener
	s.listener = nil
	s.mu.Unlock()
	if ln != nil {
		if err := ln.Close(); err != nil {
			return errors.Wrap(err, "ws Server 关闭监听失败")
		}
	}
	return nil
}

// ListenAddr 返回实际监听地址；未启动时返回错误
func (s *Server) ListenAddr() (net.Addr, error) {
	if s == nil {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil, errors.Sentinel(ErrNotStarted)
	}
	return s.listener.Addr(), nil
}
