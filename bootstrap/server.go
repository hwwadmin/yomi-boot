// Package bootstrap 提供 HTTP 服务构建与中间件装配能力
//
// @author zwei
package bootstrap

import (
	"net/http"

	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/cors"
	"gitee.com/hq_project/yomi-boot/httprecord"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/requestguard"
	"gitee.com/hq_project/yomi-boot/response"
	"gitee.com/hq_project/yomi-boot/router"
	"gitee.com/hq_project/yomi-boot/runner"
	"gitee.com/hq_project/yomi-boot/token"
	"gitee.com/hq_project/yomi-boot/traceid"

	"github.com/gin-gonic/gin"
)

// DefaultMiddlewares 返回通用 HTTP 中间件链
func DefaultMiddlewares(infra *Infra) []gin.HandlerFunc {
	corsCfg := config.MustSection[cors.Config](infra.Doc, cors.SectionKey)
	httpCfg := config.MustSection[router.Config](infra.Doc, router.SectionKey)
	httpRecordCfg := config.MustSection[httprecord.Config](infra.Doc, httprecord.SectionKey)
	authCfg := config.MustSection[token.Config](infra.Doc, token.SectionKey)
	logCfg := config.MustSection[logger.Config](infra.Doc, logger.SectionKey)

	return []gin.HandlerFunc{
		cors.Middleware(*corsCfg),
		traceid.Middleware(),
		httprecord.MiddlewareHandler(infra.HttpRecordService, *httpRecordCfg, infra.AppCtx),
		logger.AccessMiddleware(logCfg.Access),
		requestguard.Middleware(*httpCfg),
		gin.Recovery(),
		token.AuthMiddleware(infra.TokenService, *authCfg),
	}
}

// MustBuildServer 构建 HTTP 服务，失败时 panic
func MustBuildServer(infra *Infra, registrars []router.Registrar, middlewares []gin.HandlerFunc) *runner.ServerBundle {
	httpCfg := config.MustSection[router.Config](infra.Doc, router.SectionKey)

	logger.SetupGin()
	engine := router.MustNewRouter(
		httpCfg.RouterPrefixClean(),
		registrars,
		middlewares,
		httpCfg.TrustedProxies,
	)
	server := &http.Server{
		Addr:         httpCfg.ListenAddr(),
		Handler:      engine,
		ReadTimeout:  httpCfg.ReadTimeout(),
		WriteTimeout: httpCfg.WriteTimeout(),
	}
	response.MustReady()
	return &runner.ServerBundle{
		HTTPServer:      server,
		ShutdownTimeout: httpCfg.ShutdownTimeout(),
		Cleanup:         infra.Cleanup,
	}
}
