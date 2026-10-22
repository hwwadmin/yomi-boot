// Package lifecycle 编排 HTTP 服务进程生命周期
//
// Load → Init → Build 由 boot 完成；本包负责 Start → Ready → Serve → Drain → Stop → Release，
// 只处理阶段顺序、信号与优雅停机，不感知具体组件或业务。
//
// @author zwei
package lifecycle

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/banner"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
)

// Hook 是生命周期阶段钩子
type Hook = func(context.Context) error

// App 是一次 HTTP 服务进程的生命周期描述
type App struct {
	Name        string        // Name 是服务名，仅用于日志；来自 app.name
	HTTPServer  *http.Server  // HTTPServer 是已装配但尚未监听的 HTTP 服务
	OnReady     []Hook        // OnReady 在 HTTP 监听成功后按序执行；失败会导致启动失败
	OnDraining  []Hook        // OnDraining 在 HTTP 停机前逆序执行；须尊重 ctx
	StopTimeout time.Duration // StopTimeout 是 Drain + Stop 的总预算
	Release     func()        // Release 在 HTTP 停止后释放基础设施资源
}

// Run 启动服务并阻塞至进程退出；启动/运行失败时 panic
func Run(factory func() *App) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	banner.Banner()
	if err := RunContext(ctx, factory); err != nil {
		logger.ErrorWithoutCtx("服务运行失败", "error", err)
		panic(err)
	}
}
