// Package pprof 提供独立 debug 端口的性能剖析 HTTP 服务
//
// @author zwei
package pprof

import (
	"context"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

const readHeaderTimeout = 5 * time.Second

// Server 是 pprof 调试服务句柄
type Server struct {
	server   *http.Server
	listener net.Listener
}

// MustStart 启动 pprof 服务并返回句柄；未启用时返回 nil，启动失败时 log + panic
func MustStart(cfg Config) *Server {
	if !cfg.EnabledOrDefault() {
		return nil
	}

	addr := cfg.ListenAddr()
	mux := http.NewServeMux()
	registerRoutes(mux)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.ErrorWithoutCtx("pprof 监听失败", "addr", addr, "error", err)
		panic(err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	go func() {
		serveErr := server.Serve(listener)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorWithoutCtx("pprof 服务运行失败", "addr", listener.Addr().String(), "error", serveErr)
		}
	}()

	logger.InfoWithoutCtx("pprof 服务启动完成", "addr", listener.Addr().String())
	return &Server{server: server, listener: listener}
}

// Addr 返回实际监听地址；未启动时返回 nil
func (s *Server) Addr() net.Addr {
	if s == nil || s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Stop 优雅停止 pprof 服务，超时后强制关闭
func (s *Server) Stop(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	if err := s.server.Shutdown(ctx); err != nil {
		_ = s.server.Close()
		return err
	}
	return nil
}

// registerRoutes 注册 pprof 调试路由
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
