// Package boot 提供项目启动快捷入口
//
// @author zwei
package boot

import (
	"gitee.com/hq_project/yomi-boot/boot/bootstrap"
	"gitee.com/hq_project/yomi-boot/boot/lifecycle"
	"gitee.com/hq_project/yomi-boot/core/config"
	"gitee.com/hq_project/yomi-boot/core/router"

	"github.com/gin-gonic/gin"
)

// Infra 是基础设施运行时依赖聚合，等价于 bootstrap.Infra。
type Infra = bootstrap.Infra

// InfraOptions 是基础设施扩展选项，等价于 bootstrap.InfraOptions。
type InfraOptions = bootstrap.InfraOptions

// Registrar 是路由注册器，等价于 router.Registrar。
type Registrar = router.Registrar

// Middleware 是 HTTP 中间件。
type Middleware = gin.HandlerFunc

// WireFunc 在基础设施初始化完成后装配路由。
type WireFunc func(infra *Infra) []Registrar

// MiddlewareFactory 基于已初始化基础设施构建中间件链；Options.Middlewares 为空时使用 DefaultMiddlewares。
type MiddlewareFactory func(infra *Infra) []Middleware

// Options 启动选项
type Options struct {
	EnvPrefix   string            // 环境变量前缀，可为空
	ConfigPath  string            // 配置文件路径，为空时使用 config.ResolvePath()
	Infra       InfraOptions      // 基础设施扩展选项
	Middlewares MiddlewareFactory // 中间件工厂，为空时使用 DefaultMiddlewares
}

// Run 使用默认配置路径和默认中间件链启动 HTTP 服务，并阻塞运行至进程退出。
func Run(envPrefix string, wire WireFunc) {
	RunWithOptions(Options{EnvPrefix: envPrefix}, wire)
}

// RunWithOptions 初始化基础设施、构建 HTTP 服务并阻塞运行至进程退出。
func RunWithOptions(opts Options, wire WireFunc) {
	if wire == nil {
		panic("boot: WireFunc 不能为空")
	}

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = config.ResolvePath()
	}

	lifecycle.Run(func() *lifecycle.App {
		infra := bootstrap.MustInitInfra(configPath, opts.EnvPrefix, opts.Infra)
		registrars := wire(infra)

		var middlewares []Middleware
		if opts.Middlewares != nil {
			middlewares = opts.Middlewares(infra)
		} else {
			middlewares = DefaultMiddlewares(infra)
		}
		return bootstrap.MustBuildApp(infra, registrars, middlewares)
	})
}

// DefaultMiddlewares 返回通用 HTTP 中间件链。
func DefaultMiddlewares(infra *Infra) []Middleware {
	return bootstrap.DefaultMiddlewares(infra)
}
