// Package boot 提供项目启动入口与基础设施初始化。
//
// MustInit 负责配置与插件链；Run / RunWithOptions 负责编码器注册、路由装配、
// HTTP 服务构建与 lifecycle 编排。
//
// @author zwei
package boot

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/lifecycle"
	"gitee.com/hq_project/yomi-boot/boot/server"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/response"
	"gitee.com/hq_project/yomi-boot/component/core/router"
	"gitee.com/hq_project/yomi-boot/plugin"
	"gitee.com/hq_project/yomi-boot/plugin/builtin"

	"github.com/gin-gonic/gin"
)

const defaultInitTimeout = 30 * time.Second

// WireFunc 在插件初始化后装配路由。
type WireFunc func(slot *plugin.PluginSlot) []router.Registrar

// MiddlewareFactory 基于插件插槽构建 HTTP 中间件链。
type MiddlewareFactory func(slot *plugin.PluginSlot) []gin.HandlerFunc

// InitOptions 是基础设施初始化选项。
type InitOptions struct {
	ConfigPath  string               // 配置文件路径，为空时使用 config.ResolvePath()
	EnvPrefix   string               // 环境变量前缀，可为空
	Plugin      plugin.PluginOptions // 默认插件链的扩展项；PluginSet 优先
	PluginSet   *PluginSet           // 完全自定义插件链与配置段；设置后优先于默认链
	InitTimeout time.Duration        // 启动预算，0 时使用 30s；ctx 已带 deadline 时以 ctx 为准
}

// InitTimeoutOrDefault 返回启动预算，未配置时使用 30s。
func (o InitOptions) InitTimeoutOrDefault() time.Duration {
	if o.InitTimeout <= 0 {
		return defaultInitTimeout
	}
	return o.InitTimeout
}

// Options 是 HTTP 启动选项。
type Options struct {
	InitOptions
	Middlewares     MiddlewareFactory // 中间件工厂，为空时使用 server.DefaultMiddlewares
	ResponseEncoder response.Encoder  // 响应编码器，为空时使用 DefaultEncoder（已注册则沿用）
}

// PluginSet 是完全自定义插件链的装配集合，设置后只使用其 Plugins 与 Configs。
type PluginSet struct {
	// Plugins 是完整插件链，按给定顺序 Init、逆序 Close。
	Plugins []plugin.Plugin
	// Configs 是完整配置段；为空表示不注册任何配置段。
	Configs []config.Section
}

// Run 使用默认配置和默认中间件启动 HTTP 服务，阻塞至进程退出。
func Run(envPrefix string, wire WireFunc) {
	RunWithOptions(Options{InitOptions: InitOptions{EnvPrefix: envPrefix}}, wire)
}

// RunWithOptions 按 opts 初始化基础设施并启动 HTTP 服务，阻塞至进程退出。
// 未指定中间件时使用默认中间件链。
func RunWithOptions(opts Options, wire WireFunc) {
	if wire == nil {
		panic("boot: WireFunc 不能为空")
	}

	// 注册响应编码器
	registerResponseEncoder(opts.ResponseEncoder)

	// 解析配置文件路径
	if opts.ConfigPath == "" {
		opts.ConfigPath = config.ResolvePath()
	}

	lifecycle.Run(func() *lifecycle.App {
		// 初始化基础设施：注册配置段、加载校验、按序执行插件
		slot := MustInit(context.Background(), opts.InitOptions)
		defer func() {
			if r := recover(); r != nil {
				_ = slot.Close(context.Background())
				panic(r)
			}
		}()

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

// registerResponseEncoder 注册响应编码器；未显式指定时复用已注册实现或注册 DefaultEncoder。
func registerResponseEncoder(enc response.Encoder) {
	if enc != nil {
		response.MustRegister(enc)
		return
	}
	if !response.Registered() {
		response.MustRegister(response.DefaultEncoder{})
	}
}

// MustInit 初始化配置与插件链并返回插件插槽，初始化失败时 panic。
// ctx 控制启动预算；未携带 deadline 时使用 opts.InitTimeout（默认 30s）。
// PluginSet 非空时使用自定义插件与配置段，否则使用内置默认链并追加 Plugin 扩展。
func MustInit(ctx context.Context, opts InitOptions) *plugin.PluginSlot {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.InitTimeoutOrDefault())
		defer cancel()
	}

	reg := config.NewRegistry()
	var plugins []plugin.Plugin
	if opts.PluginSet != nil {
		reg.Register(opts.PluginSet.Configs...)
		plugins = opts.PluginSet.Plugins
	} else {
		reg.Register(builtin.DefaultConfigs()...)
		reg.Register(opts.Plugin.CustomConfigs()...)
		plugins = append(builtin.DefaultPlugins(), opts.Plugin.CustomPlugins()...)
	}

	// 加载配置文件
	cfg := config.MustLoadConfig(opts.ConfigPath, opts.EnvPrefix, reg)

	// 创建插件插槽并初始化插件
	slot := plugin.NewPluginSlot(cfg, plugins)
	if err := slot.Init(ctx); err != nil {
		panic(err)
	}
	return slot
}
