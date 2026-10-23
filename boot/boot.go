// Package boot 提供项目启动快捷入口与基础设施初始化。
//
// MustInit 注册配置段、加载配置文档并按内置插件清单初始化插件链；Run / RunWithOptions
// 在此基础上完成响应编码器注册、路由装配、HTTP 服务构建与 lifecycle 编排。
//
// @author zwei
package boot

import (
	"gitee.com/hq_project/yomi-boot/boot/lifecycle"
	"gitee.com/hq_project/yomi-boot/boot/server"
	"gitee.com/hq_project/yomi-boot/component/common/router"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/response"
	"gitee.com/hq_project/yomi-boot/plugin"
	"gitee.com/hq_project/yomi-boot/plugin/registry"

	"github.com/gin-gonic/gin"
)

// WireFunc 在基础设施初始化完成后装配路由。
type WireFunc func(slot *plugin.PluginSlot) []router.Registrar

// MiddlewareFactory 基于已初始化的插件链构建中间件链；Options.Middlewares 为空时使用 server.DefaultMiddlewares。
type MiddlewareFactory func(slot *plugin.PluginSlot) []gin.HandlerFunc

// Options 启动选项
type Options struct {
	EnvPrefix       string               // 环境变量前缀，可为空
	ConfigPath      string               // 配置文件路径，为空时使用 config.ResolvePath()
	Plugin          plugin.PluginOptions // 插件扩展选项（自定义插件 / 自定义配置段）
	Middlewares     MiddlewareFactory    // 中间件工厂，为空时使用 server.DefaultMiddlewares
	ResponseEncoder response.Encoder     // 响应编码器，为空时注册 response.DefaultEncoder{}（业务已提前注册则沿用）
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

	// 注册响应编码器
	registerResponseEncoder(opts.ResponseEncoder)

	// 解析配置文件路径
	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = config.ResolvePath()
	}

	lifecycle.Run(func() *lifecycle.App {
		// 初始化基础设施：注册配置段、加载校验、按序执行插件
		slot := MustInit(configPath, opts.EnvPrefix, opts.Plugin)

		// 装配路由
		registrars := wire(slot)

		// 构建中间件链
		var middlewares []gin.HandlerFunc
		if opts.Middlewares != nil {
			middlewares = opts.Middlewares(slot)
		} else {
			middlewares = server.DefaultMiddlewares(slot)
		}

		// 构建 HTTP 服务
		return server.MustBuildApp(slot, registrars, middlewares)
	})
}

// registerResponseEncoder 注册响应编码器：显式指定时使用指定实现；
// 未指定且业务也未提前注册时使用 DefaultEncoder，已提前注册则不重复注册。
func registerResponseEncoder(enc response.Encoder) {
	if enc != nil {
		response.MustRegister(enc)
		return
	}
	if !response.Registered() {
		response.MustRegister(response.DefaultEncoder{})
	}
}

// MustInit 初始化基础设施：注册配置段、加载校验、按序执行插件；失败时 panic。
// 返回插件插槽；进程退出时由 lifecycle 调用 slot.Close 逆序释放资源。
func MustInit(configPath, envPrefix string, pluginOpts plugin.PluginOptions) *plugin.PluginSlot {
	// 注册配置段
	reg := registry.DefaultConfigRegistry()
	for _, section := range pluginOpts.CustomConfigs() {
		reg.Register(section)
	}

	// 加载配置文件
	cfg := config.MustLoadConfig(configPath, envPrefix, reg)

	// 创建插件插槽并初始化插件
	slot := plugin.NewPluginSlot(cfg, append(registry.Plugins(), pluginOpts.CustomPlugins()...))
	slot.Init()

	return slot
}
