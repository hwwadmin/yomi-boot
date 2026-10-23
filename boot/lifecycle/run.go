package lifecycle

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

const defaultStopTimeout = 10 * time.Second

// RunContext 使用调用方提供的 ctx 启动服务，返回错误而不 panic；ctx 取消时执行优雅停机。
func RunContext(ctx context.Context, factory func() *App) (runErr error) {
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

	name := app.Name
	if name == "" {
		name = "service"
	}
	stopTimeout := app.StopTimeout
	if stopTimeout <= 0 {
		stopTimeout = defaultStopTimeout
	}
	releaseTimeout := app.ReleaseTimeout
	if releaseTimeout <= 0 {
		releaseTimeout = defaultStopTimeout
	}

	release := onceRelease(app.Release, releaseTimeout)
	defer func() {
		if err := release(); err != nil {
			logger.ErrorWithoutCtx(name+" 释放资源失败", "error", err)
			runErr = errors.Join(runErr, err)
		}
	}()

	if app.HTTPServer == nil {
		return errors.New("lifecycle: HTTPServer 不能为空")
	}
	if app.HTTPServer.Addr == "" {
		return errors.New("lifecycle: HTTPServer.Addr 不能为空")
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
			return errors.Join(err, shutdown(name, app, stopTimeout))
		}
		logger.InfoWithoutCtx(name+" 服务就绪", "addr", listener.Addr().String())
	}

	select {
	case runErr = <-serveErr:
		if runErr != nil {
			logger.ErrorWithoutCtx(name+" HTTP 运行失败", "error", runErr)
		}
	case <-ctx.Done():
		logger.InfoWithoutCtx(name + " 收到退出请求")
	}

	return errors.Join(runErr, shutdown(name, app, stopTimeout))
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
		if err := runHook(hook, ctx); err != nil {
			return err
		}
	}
	return nil
}

// runHook 执行单个 hook，并把 panic 转为 error
func runHook(hook Hook, ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Newf("hook panic: %v", r)
		}
	}()
	return hook(ctx)
}

// shutdown 逆序执行排空钩子，随后停止 HTTP 服务，返回聚合错误
func shutdown(name string, app *App, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var errs []error
	for i := len(app.OnDraining) - 1; i >= 0; i-- {
		hook := app.OnDraining[i]
		if hook == nil {
			continue
		}
		if err := runHook(hook, ctx); err != nil {
			logger.ErrorWithoutCtx(name+" 排空钩子失败", "error", err)
		}
	}

	if err := app.HTTPServer.Shutdown(ctx); err != nil {
		logger.ErrorWithoutCtx(name+" HTTP 优雅停机失败", "error", err)
		_ = app.HTTPServer.Close()
		errs = append(errs, errors.Wrap(err, "HTTP 优雅停机失败"))
	}
	logger.InfoWithoutCtx(name + " HTTP 已停止")
	return errors.Join(errs...)
}

// onceRelease 保证 Release 只执行一次，并返回其错误
func onceRelease(release func(context.Context) error, timeout time.Duration) func() error {
	var once sync.Once
	var err error
	return func() error {
		once.Do(func() {
			if release == nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			err = release(ctx)
		})
		return err
	}
}
