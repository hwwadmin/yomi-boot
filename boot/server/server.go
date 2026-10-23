// HTTP 服务与生命周期描述构建。

package server

import (
	"net/http"

	"gitee.com/hq_project/yomi-boot/boot/lifecycle"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/router"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/response"
	"gitee.com/hq_project/yomi-boot/plugin"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	routerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/router"

	"github.com/gin-gonic/gin"
)

// MustBuildApp 构建 HTTP 服务与生命周期描述，失败时 panic
func MustBuildApp(slot *plugin.PluginSlot, registrars []router.Registrar, middlewares []gin.HandlerFunc) *lifecycle.App {
	if slot == nil {
		panic("server: PluginSlot 不能为空")
	}
	response.MustReady()
	httpCfg := config.MustSection[routerplugin.BootConfig](slot.Config, routerplugin.SectionKey).ToConfig()

	logger.SetupGin()
	engine := router.MustNewRouter(
		httpCfg.RouterPrefixClean(),
		registrars,
		middlewares,
		httpCfg.TrustedProxies,
	)
	server := &http.Server{
		Addr:              httpCfg.ListenAddr(),
		Handler:           engine,
		ReadTimeout:       httpCfg.ReadTimeout(),
		WriteTimeout:      httpCfg.WriteTimeout(),
		ReadHeaderTimeout: httpCfg.ReadHeaderTimeout(),
	}

	return &lifecycle.App{
		Name:        serviceName(slot),
		HTTPServer:  server,
		OnReady:     slot.ReadyHooks(),
		OnDraining:  slot.DrainHooks(),
		StopTimeout: httpCfg.ShutdownTimeout(),
		Release:     slot.Close,
	}
}

// serviceName 返回服务名；app 插件未初始化时返回空串
func serviceName(slot *plugin.PluginSlot) string {
	app := appctxplugin.From(slot).AppCtx()
	if app == nil {
		return ""
	}
	return app.Name()
}
