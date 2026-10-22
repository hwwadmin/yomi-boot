// Package boot 提供项目启动快捷入口与基础设施初始化。
//
// MustInit 注册配置段、加载配置文档并按内置插件清单启动资源；Run / RunWithOptions
// 在此基础上完成路由装配、HTTP 服务构建与 lifecycle 编排。
//
// @author zwei
package boot

import (
	"gitee.com/hq_project/yomi-boot/boot/lifecycle"
	"gitee.com/hq_project/yomi-boot/boot/server"
	"gitee.com/hq_project/yomi-boot/component/common/router"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
	"gitee.com/hq_project/yomi-boot/plugin/registry"

	"github.com/gin-gonic/gin"
)

// WireFunc 在基础设施初始化完成后装配路由。
type WireFunc func(services *plugin.Services) []router.Registrar

// MiddlewareFactory 基于已初始化能力构建中间件链；Options.Middlewares 为空时使用 server.DefaultMiddlewares。
type MiddlewareFactory func(services *plugin.Services) []gin.HandlerFunc

// Options 启动选项
type Options struct {
	EnvPrefix   string               // 环境变量前缀，可为空
	ConfigPath  string               // 配置文件路径，为空时使用 config.ResolvePath()
	Plugin      plugin.PluginOptions // 插件扩展选项（自定义插件 / 自定义配置段）
	Middlewares MiddlewareFactory    // 中间件工厂，为空时使用 server.DefaultMiddlewares
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
		services, rt := MustInit(configPath, opts.EnvPrefix, opts.Plugin)
		registrars := wire(services)

		var middlewares []gin.HandlerFunc
		if opts.Middlewares != nil {
			middlewares = opts.Middlewares(services)
		} else {
			middlewares = server.DefaultMiddlewares(services)
		}
		return server.MustBuildApp(services, rt, registrars, middlewares)
	})
}

// MustInit 初始化基础设施：注册配置段、加载校验、按序执行插件；失败时 panic。
// 返回业务可见的能力集合与插件运行时；进程退出时由调用方关闭 Runtime。
func MustInit(configPath, envPrefix string, pluginOpts plugin.PluginOptions) (*plugin.Services, *plugin.Runtime) {
	reg := registry.DefaultConfigRegistry()
	for _, section := range pluginOpts.CustomConfigs() {
		reg.Register(section)
	}
	cfg := config.MustLoadConfig(configPath, envPrefix, reg)

	host := &plugin.Host{Services: plugin.Services{Config: cfg}}
	rt := plugin.Run(host, append(registry.Plugins(), pluginOpts.CustomPlugins()...))
	return &host.Services, rt
}
