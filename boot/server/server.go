// HTTP 服务与生命周期描述构建。

package server

import (
	"net/http"

	"gitee.com/hq_project/yomi-boot/boot/lifecycle"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/response"
	"gitee.com/hq_project/yomi-boot/component/common/router"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
	routerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/router"

	"github.com/gin-gonic/gin"
)

// MustBuildApp 构建 HTTP 服务与生命周期描述，失败时 panic
func MustBuildApp(services *plugin.Services, rt *plugin.Runtime, registrars []router.Registrar, middlewares []gin.HandlerFunc) *lifecycle.App {
	if services == nil {
		panic("server: Services 不能为空")
	}
	response.MustReady()
	httpCfg := config.MustSection[routerplugin.BootConfig](services.Doc, routerplugin.SectionKey).ToConfig()

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
		Name:        serviceName(services),
		HTTPServer:  server,
		OnReady:     rt.ReadyHooks(),
		OnDraining:  rt.DrainHooks(),
		StopTimeout: httpCfg.ShutdownTimeout(),
		Release:     rt.Close,
	}
}

// serviceName 返回服务名；AppCtx 未初始化时返回空串
func serviceName(services *plugin.Services) string {
	if services.AppCtx == nil {
		return ""
	}
	return services.AppCtx.Name()
}
