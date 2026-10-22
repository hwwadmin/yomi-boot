// Package bootstrap 提供 HTTP 服务构建与中间件装配能力
//
// @author zwei
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
	httprecordplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/httprecord"
	loggerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/logger"
	tokenplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/token"

	"github.com/gin-gonic/gin"
)

// DefaultMiddlewares 返回通用 HTTP 中间件链
func DefaultMiddlewares(infra *Infra) []gin.HandlerFunc {
	corsCfg := config.MustSection[cors.Config](infra.Doc, cors.SectionKey)
	httpCfg := config.MustSection[router.Config](infra.Doc, router.SectionKey)
	httpRecordCfg := config.MustSection[httprecordplugin.BootConfig](infra.Doc, httprecordplugin.SectionKey).ToConfig()
	authCfg := config.MustSection[tokenplugin.BootConfig](infra.Doc, tokenplugin.SectionKey).ToConfig()
	logCfg := config.MustSection[loggerplugin.BootConfig](infra.Doc, loggerplugin.SectionKey).ToConfig()

	return []gin.HandlerFunc{
		cors.Middleware(*corsCfg),
		traceid.Middleware(),
		httprecord.MiddlewareHandler(infra.HttpRecordService, httpRecordCfg, infra.AppCtx),
		logger.AccessMiddleware(logCfg.Access),
		httpguard.Middleware(*httpCfg),
		gin.Recovery(),
		token.AuthMiddleware(infra.TokenService, authCfg),
	}
}

// MustBuildApp 构建 HTTP 服务与生命周期描述，失败时 panic
func MustBuildApp(infra *Infra, registrars []router.Registrar, middlewares []gin.HandlerFunc) *lifecycle.App {
	if infra == nil {
		panic("bootstrap: Infra 不能为空")
	}
	response.MustReady()
	httpCfg := config.MustSection[router.Config](infra.Doc, router.SectionKey)

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

	app := &lifecycle.App{
		Name:        infraName(infra),
		HTTPServer:  server,
		StopTimeout: httpCfg.ShutdownTimeout(),
		Release:     infra.Cleanup,
	}
	if infra.runtime != nil {
		app.OnReady = infra.runtime.ReadyHooks()
		app.OnDraining = infra.runtime.DrainHooks()
	}
	return app
}

// infraName 返回服务名；AppCtx 未初始化时返回空串
func infraName(infra *Infra) string {
	if infra.AppCtx == nil {
		return ""
	}
	return infra.AppCtx.Name()
}
