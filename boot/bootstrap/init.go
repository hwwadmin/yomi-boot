// MustInit 加载配置并按内置插件链启动基础设施。

package bootstrap

import (
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
	"gitee.com/hq_project/yomi-boot/plugin/registry"
)

// MustInit 初始化基础设施：注册配置段、加载校验、按序执行插件；失败时 panic。
// 返回业务可见的能力集合与插件运行时；进程退出时由调用方关闭 Runtime。
func MustInit(configPath, envPrefix string, opts InitOptions) (*plugin.Services, *plugin.Runtime) {
	reg := registry.DefaultConfigRegistry()
	for _, s := range opts.configSections {
		reg.Register(s)
	}
	doc := config.MustLoadDocument(configPath, envPrefix, reg)

	host := &plugin.Host{Services: plugin.Services{Doc: doc}}
	rt := plugin.Run(host, append(registry.Plugins(), opts.customPlugins...))
	return &host.Services, rt
}
