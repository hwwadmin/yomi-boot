// HTTP 服务构建与中间件装配。

package bootstrap

import (
	"net/http"

	"gitee.com/hq_project/yomi-boot/boot/lifecycle"
	"gitee.com/hq_project/yomi-boot/component/common/httpguard"
	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/response"
	"gitee.com/hq_project/yomi-boot/component/common/router"
	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/cors"
	"gitee.com/hq_project/yomi-boot/component/core/traceid"
	"gitee.com/hq_project/yomi-boot/plugin"
	corsplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/cors"
	httprecordplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/httprecord"
	loggerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/logger"
	routerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/router"
	tokenplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/token"

	"github.com/gin-gonic/gin"
)

// DefaultMiddlewares 返回通用 HTTP 中间件链
func DefaultMiddlewares(services *plugin.Services) []gin.HandlerFunc {
	corsCfg := config.MustSection[corsplugin.BootConfig](services.Doc, corsplugin.SectionKey).ToConfig()
	httpCfg := config.MustSection[routerplugin.BootConfig](services.Doc, routerplugin.SectionKey).ToConfig()
	httpRecordCfg := config.MustSection[httprecordplugin.BootConfig](services.Doc, httprecordplugin.SectionKey).ToConfig()
	authCfg := config.MustSection[tokenplugin.BootConfig](services.Doc, tokenplugin.SectionKey).ToConfig()
	logCfg := config.MustSection[loggerplugin.BootConfig](services.Doc, loggerplugin.SectionKey).ToConfig()

	return []gin.HandlerFunc{
		cors.Middleware(corsCfg),
		traceid.Middleware(),
		httprecord.Middleware(services.HttpRecordService, httpRecordCfg, services.AppCtx),
		logger.Middleware(logCfg.Access),
		httpguard.Middleware(httpCfg),
		gin.Recovery(),
		token.Middleware(services.TokenService, authCfg),
	}
}

// MustBuildApp 构建 HTTP 服务与生命周期描述，失败时 panic
func MustBuildApp(services *plugin.Services, rt *plugin.Runtime, registrars []router.Registrar, middlewares []gin.HandlerFunc) *lifecycle.App {
	if services == nil {
		panic("bootstrap: Services 不能为空")
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
