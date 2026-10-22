// Package ws 实现独立端口 WebSocket Server
//
// @author zwei
package ws

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/google/uuid"
)

// Server 是入站服务：按 ConnID 推送
type Server interface {
	// Start 非阻塞启动监听；TLS 由 ServerConfig 决定；ctx 取消时关闭
	Start(ctx context.Context) error
	// Close 关闭监听与全部会话，并停止队列巡检
	Close(ctx context.Context) error
	// Send 向指定 ConnID 入队发送；成功表示已入队
	Send(connID string, typ MsgType, data []byte) error
	// Broadcast 并行入队广播；已拆连接跳过；有界满队时可能阻塞
	Broadcast(typ MsgType, data []byte) error
	// Len 在线会话数
	Len() int
	// Has ConnID 是否在线
	Has(connID string) bool
	// ListenAddr 实际监听地址
	ListenAddr() (net.Addr, error)
}

// server 是 Server 的默认实现
type server struct {
	addr        string        // addr 是配置的监听地址
	handler     ServerHandler // handler 是会话回调
	tlsEnabled  bool          // tlsEnabled 是否启用 TLS
	tlsCertFile string        // tlsCertFile 证书路径
	tlsKeyFile  string        // tlsKeyFile 私钥路径
	tlsConfig   *tls.Config   // tlsConfig 可选合并配置

	handshakeTimeout      time.Duration            // handshakeTimeout 已 resolve 的握手超时
	maxConnLifetime       time.Duration            // maxConnLifetime 已 resolve 的通道最大存活；0 表示不限制
	maxConnLifetimeJitter time.Duration            // maxConnLifetimeJitter 已 resolve 的波动半宽
	maxConnections        int64                    // maxConnections 在线上限；0 表示不限制
	heartbeat             heartbeatSettings        // heartbeat 已 resolve 的心跳
	writeMax              int                      // writeMax 单条可写上限
	readMax               int                      // readMax 单条可读上限
	maxFrame              int                      // maxFrame 出站分片目标帧载荷；0 关闭
	sendQueueSize         int                      // sendQueueSize 每连接发送队列容量；0 无界
	queueGCInterval       time.Duration            // queueGCInterval 巡检间隔；0 关闭巡检
	compress              bool                     // compress 是否启用压缩
	originPatterns        []string                 // originPatterns 允许的 Origin；空则跳过校验
	authorize             func(*http.Request) bool // authorize 握手鉴权

	httpSrv     *http.Server // httpSrv 承接 Accept
	listener    net.Listener // listener 是当前监听器
	hub         *hub         // hub 是 ConnID 表
	closed      atomic.Bool  // closed 表示已关闭
	started     atomic.Bool  // started 表示已启动
	activeConns atomic.Int64 // activeConns 当前已占槽的在线连接数

	gcStop     chan struct{} // gcStop 停止队列巡检
	gcStopOnce sync.Once     // gcStopOnce 保证只关一次
}

// NewServer 创建入站服务；ConnID 由框架在连通时自动生成
func NewServer(cfg ServerConfig) (Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	interval, timeout := cfg.ResolvedHeartbeat()
	lifetime, jitter := cfg.ResolvedMaxConnLifetime()
	return &server{
		addr:                  cfg.Addr,
		handler:               cfg.resolveHandler(),
		tlsEnabled:            cfg.TLSEnabled,
		tlsCertFile:           cfg.TLSCertFile,
		tlsKeyFile:            cfg.TLSKeyFile,
		tlsConfig:             cfg.TLSConfig,
		handshakeTimeout:      cfg.ResolvedHandshakeTimeout(),
		maxConnLifetime:       lifetime,
		maxConnLifetimeJitter: jitter,
		maxConnections:        int64(cfg.MaxConnections),
		heartbeat:             heartbeatFromResolved(interval, timeout),
		writeMax:              cfg.ResolvedWriteMaxPayloadSize(),
		readMax:               cfg.ResolvedReadMaxPayloadSize(),
		maxFrame:              cfg.MaxFramePayloadSize,
		sendQueueSize:         cfg.SendQueueSize,
		queueGCInterval:       cfg.ResolvedQueueGCInterval(),
		compress:              cfg.CompressEnabled,
		originPatterns:        append([]string(nil), cfg.OriginPatterns...),
		authorize:             cfg.Authorize,
		hub:                   newHub(),
		gcStop:                make(chan struct{}),
	}, nil
}

// Start 非阻塞启动监听（明文或 TLS 由配置决定）
func (s *server) Start(ctx context.Context) error {
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
	if s.tlsEnabled {
		cert, err := tls.LoadX509KeyPair(s.tlsCertFile, s.tlsKeyFile)
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

	hs := &http.Server{
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: s.handshakeTimeout,
	}

	s.hub.mu.Lock()
	if s.closed.Load() {
		s.hub.mu.Unlock()
		_ = ln.Close()
		s.started.Store(false)
		return errors.Sentinel(ErrClosed)
	}
	s.listener = ln
	s.httpSrv = hs
	s.hub.mu.Unlock()

	go func() {
		_ = hs.Serve(ln)
	}()
	s.startQueueGC()
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = s.Close(context.Background())
		}()
	}
	return nil
}

// serveHTTP 完成握手、登记会话并阻塞运行读循环
func (s *server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if s.closed.Load() {
		http.Error(w, "server closing", http.StatusServiceUnavailable)
		return
	}
	if s.authorize != nil && !s.authorize(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ref := &connRef{}
	raw, err := acceptRaw(w, r, acceptOptions(s.originPatterns, s.compress, ref))
	if err != nil {
		return
	}

	s.hub.mu.Lock()
	local := net.Addr(nil)
	if s.listener != nil {
		local = s.listener.Addr()
	}
	s.hub.mu.Unlock()

	conn := newConn(connParams{
		raw:           raw,
		remote:        parseNetAddr(r.RemoteAddr),
		local:         local,
		writeMax:      s.writeMax,
		readMax:       s.readMax,
		maxFrame:      s.maxFrame,
		sendQueueSize: s.sendQueueSize,
	})
	ref.bind(conn)
	conn.onPayload = func(c *connImpl, p Payload) {
		s.handler.OnPayload(&sessionContext{peerContext: peerContext{ctx: context.Background(), conn: c}}, p)
	}
	conn.onClose = func(c *connImpl, closeErr error) {
		s.hub.unregister(c.connID, c)
		s.handler.OnDisconnect(c.connID, closeErr)
	}

	if s.closed.Load() {
		_ = conn.Close(CloseGoingAway, "server closing")
		return
	}
	if !s.tryAcquireConn() {
		_ = conn.Close(CloseTryAgainLater, "max connections exceeded")
		return
	}
	defer s.releaseConn()

	connID := uuid.NewString()
	if !s.tryRegister(connID, conn) {
		_ = conn.Close(CloseGoingAway, "server closing")
		return
	}
	conn.connID = connID

	// OnConnect 等 panic 时拆除已登记连接
	enteredServe := false
	defer func() {
		if enteredServe {
			return
		}
		if recover() != nil {
			s.hub.unregister(connID, conn)
			_ = conn.Close(CloseGoingAway, "connect setup panic")
		}
	}()
	s.handler.OnConnect(&sessionContext{peerContext: peerContext{ctx: context.Background(), conn: conn}})
	conn.startHeartbeat(s.heartbeat)
	conn.startLifetime(s.maxConnLifetime, s.maxConnLifetimeJitter)
	enteredServe = true
	conn.serve()
}

// tryRegister 在未关闭时登记连接
func (s *server) tryRegister(id string, c *connImpl) bool {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	if s.closed.Load() {
		return false
	}
	s.hub.putLocked(id, c)
	return true
}

// tryAcquireConn 尝试占一个连接槽；MaxConnections==0 时不限制但仍计数
func (s *server) tryAcquireConn() bool {
	if s.maxConnections <= 0 {
		s.activeConns.Add(1)
		return true
	}
	for {
		n := s.activeConns.Load()
		if n >= s.maxConnections {
			return false
		}
		if s.activeConns.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// releaseConn 释放一个连接槽
func (s *server) releaseConn() {
	s.activeConns.Add(-1)
}

// Send 向指定 ConnID 入队发送；服务已关闭返回 ErrClosed
func (s *server) Send(connID string, typ MsgType, data []byte) error {
	if s == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if s.closed.Load() {
		return errors.Sentinel(ErrClosed)
	}
	if connID == "" {
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "connID 不能为空")
	}
	conn, ok := s.hub.get(connID)
	if !ok || conn == nil {
		if s.closed.Load() {
			return errors.Sentinel(ErrClosed)
		}
		return errors.Sentinel(ErrUnknownPeer)
	}
	if s.closed.Load() {
		return errors.Sentinel(ErrClosed)
	}
	err := conn.enqueue(typ, data)
	if errors.Is(err, ErrNotConnected) {
		if s.closed.Load() {
			return errors.Sentinel(ErrClosed)
		}
		return errors.Sentinel(ErrUnknownPeer)
	}
	return err
}

// Broadcast 并行入队广播；已关闭返回 ErrClosed
func (s *server) Broadcast(typ MsgType, data []byte) error {
	if s == nil || s.hub == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if s.closed.Load() {
		return errors.Sentinel(ErrClosed)
	}
	switch typ {
	case MsgText, MsgBinary:
		err := s.hub.broadcast(typ, data)
		if s.closed.Load() {
			return errors.Sentinel(ErrClosed)
		}
		return err
	default:
		return errors.Wrap(errors.Sentinel(ErrInvalidArgument), "不支持的 MsgType")
	}
}

// Len 返回在线会话数
func (s *server) Len() int {
	if s == nil || s.hub == nil {
		return 0
	}
	return s.hub.len()
}

// Has 判断 ConnID 是否在线
func (s *server) Has(connID string) bool {
	if s == nil || s.hub == nil {
		return false
	}
	_, ok := s.hub.get(connID)
	return ok
}

// Close 拒新入队，停听与巡检，再关掉已登记会话
func (s *server) Close(_ context.Context) error {
	if s == nil {
		return nil
	}
	s.hub.mu.Lock()
	if !s.closed.CompareAndSwap(false, true) {
		s.hub.mu.Unlock()
		return nil
	}
	list := s.hub.takeAllLocked()
	hs := s.httpSrv
	s.httpSrv = nil
	s.listener = nil
	s.hub.mu.Unlock()

	s.stopQueueGC()

	var closeErr error
	if hs != nil {
		if err := hs.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			closeErr = errors.Wrap(err, "ws Server 关闭监听失败")
		}
	}
	for _, c := range list {
		_ = c.Close(CloseGoingAway, "server closing")
	}
	return closeErr
}

// ListenAddr 返回实际监听地址；未启动时返回错误
func (s *server) ListenAddr() (net.Addr, error) {
	if s == nil {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	if s.listener == nil {
		return nil, errors.Sentinel(ErrNotStarted)
	}
	return s.listener.Addr(), nil
}

// startQueueGC 启动孤儿队列巡检；间隔为 0 时不启动
func (s *server) startQueueGC() {
	if s == nil || s.queueGCInterval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(s.queueGCInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.gcStop:
				return
			case <-ticker.C:
				s.purgeDeadQueues()
			}
		}
	}()
}

// stopQueueGC 停止巡检
func (s *server) stopQueueGC() {
	if s == nil {
		return
	}
	s.gcStopOnce.Do(func() {
		if s.gcStop != nil {
			close(s.gcStop)
		}
	})
}

// purgeDeadQueues 清理已关闭连接上残留的发送队列
func (s *server) purgeDeadQueues() {
	if s == nil || s.hub == nil {
		return
	}
	for _, c := range s.hub.snapshot() {
		if c == nil || c.sendQ == nil {
			continue
		}
		if c.sendQ.isClosed() {
			c.sendQ.purgeIfDead()
		}
	}
}
