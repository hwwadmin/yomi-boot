// Package runner 提供服务运行与优雅停机能力
//
// @author zwei
package runner

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/appctx"
	"gitee.com/hq_project/yomi-boot/boot/banner"
	"gitee.com/hq_project/yomi-boot/boot/config"
	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/logger"
)

// ServerBundle 表示入口启动完成后的 HTTP 服务和资源清理器
type ServerBundle struct {
	HTTPServer      *http.Server  // HTTPServer 是当前入口的 HTTP 服务实例
	ShutdownTimeout time.Duration // ShutdownTimeout 是优雅停机超时时间
	Cleanup         func()        // Cleanup 用于释放启动阶段创建的资源
}

// Run 使用默认配置路径启动 HTTP 服务
func Run(name string, factory func(string) *ServerBundle) {
	RunServer(name, func() *ServerBundle {
		return factory(config.ResolvePath())
	})
}

// RunServer 启动 HTTP 服务并处理优雅停机，name 用于日志标识，factory 用于创建服务实例
func RunServer(name string, factory func() *ServerBundle) {
	banner.Banner()

	bundle := factory()
	defer closeBundle(name, bundle)

	errCh := make(chan error, 1)
	go func() {
		serveErr := bundle.HTTPServer.ListenAndServe()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	logger.InfoWithoutCtx(name+"服务启动完成", "addr", bundle.HTTPServer.Addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil {
			logger.ErrorWithoutCtx(name+"服务运行失败", "error", err)
			panic(err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := appctx.BackgroundWithTimeout(bundle.ShutdownTimeout)
		defer cancel()
		if err := bundle.HTTPServer.Shutdown(shutdownCtx); err != nil {
			logger.ErrorWithoutCtx(name+"HTTP 优雅停机失败", "error", err)
		}
	}
}

// closeBundle 释放启动阶段创建的基础资源，name 用于日志标识，bundle 为 nil 或 Cleanup 为 nil 时静默跳过
func closeBundle(name string, bundle *ServerBundle) {
	if bundle == nil || bundle.Cleanup == nil {
		return
	}
	bundle.Cleanup()
}
