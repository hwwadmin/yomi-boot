package lifecycle

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

const defaultStopTimeout = 10 * time.Second

// RunContext 使用调用方提供的 ctx 启动服务，返回错误而不 panic；ctx 取消时执行优雅停机
func RunContext(ctx context.Context, factory func() *App) error {
	ctx = appctx.OrBackground(ctx)
	if ctx.Err() != nil {
		return nil
	}
	if factory == nil {
		return errors.New("lifecycle: factory 不能为空")
	}

	app := factory()
	if app == nil {
		return errors.New("lifecycle: App 不能为空")
	}
	release := onceRelease(app.Release)
	defer release()

	if app.HTTPServer == nil {
		return errors.New("lifecycle: HTTPServer 不能为空")
	}
	if app.HTTPServer.Addr == "" {
		return errors.New("lifecycle: HTTPServer.Addr 不能为空")
	}

	name := app.Name
	if name == "" {
		name = "service"
	}
	stopTimeout := app.StopTimeout
	if stopTimeout <= 0 {
		stopTimeout = defaultStopTimeout
	}

	if ctx.Err() != nil {
		logger.InfoWithoutCtx(name + " 启动已取消")
		return nil
	}

	listener, err := net.Listen("tcp", app.HTTPServer.Addr)
	if err != nil {
		return errors.Wrap(err, "HTTP 监听失败")
	}
	logger.InfoWithoutCtx(name+" HTTP 已监听", "addr", listener.Addr().String())

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- normalizeServeError(app.HTTPServer.Serve(listener))
	}()

	if ctx.Err() == nil {
		if err := runReadyHooks(ctx, app.OnReady); err != nil {
			logger.ErrorWithoutCtx(name+" 就绪钩子失败", "error", err)
			shutdown(name, app, stopTimeout)
			return err
		}
		logger.InfoWithoutCtx(name+" 服务就绪", "addr", listener.Addr().String())
	}

	var runErr error
	select {
	case runErr = <-serveErr:
		if runErr != nil {
			logger.ErrorWithoutCtx(name+" HTTP 运行失败", "error", runErr)
		}
	case <-ctx.Done():
		logger.InfoWithoutCtx(name + " 收到退出请求")
	}

	shutdown(name, app, stopTimeout)
	return runErr
}

// normalizeServeError 将 http.ErrServerClosed 归一化为 nil
func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// runReadyHooks 按序执行就绪钩子，遇错立即返回
func runReadyHooks(ctx context.Context, hooks []Hook) error {
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(ctx); err != nil {
			return err
		}
	}
	return nil
}

// shutdown 逆序执行排空钩子，随后停止 HTTP 服务
func shutdown(name string, app *App, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for i := len(app.OnDraining) - 1; i >= 0; i-- {
		hook := app.OnDraining[i]
		if hook == nil {
			continue
		}
		if err := hook(ctx); err != nil {
			logger.ErrorWithoutCtx(name+" 排空钩子失败", "error", err)
		}
	}

	if err := app.HTTPServer.Shutdown(ctx); err != nil {
		logger.ErrorWithoutCtx(name+" HTTP 优雅停机失败", "error", err)
		_ = app.HTTPServer.Close()
	}
	logger.InfoWithoutCtx(name + " HTTP 已停止")
}

// onceRelease 保证 Release 只执行一次
func onceRelease(release func()) func() {
	var once sync.Once
	return func() {
		if release != nil {
			once.Do(release)
		}
	}
}
