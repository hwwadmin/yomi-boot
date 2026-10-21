// Package pprof 提供独立 debug 端口的性能剖析 HTTP 服务
//
// @author zwei
package pprof

import (
	"net"
	"net/http"
	"net/http/pprof"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/logger"
)

// MustStart 启动 pprof 服务，失败时 log + panic；进程退出时随进程自然关闭，无需显式释放
func MustStart(cfg Config) {
	if !cfg.EnabledOrDefault() {
		return
	}

	addr := cfg.ListenAddr()
	mux := http.NewServeMux()
	registerRoutes(mux)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.ErrorWithoutCtx("pprof 监听失败", "addr", addr, "error", err)
		panic(err)
	}

	go func() {
		serveErr := http.Serve(listener, mux)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorWithoutCtx("pprof 服务运行失败", "addr", addr, "error", serveErr)
		}
	}()

	logger.InfoWithoutCtx("pprof 服务启动完成", "addr", addr)
}

// registerRoutes 注册 pprof 调试路由
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
