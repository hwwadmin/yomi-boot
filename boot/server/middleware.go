// HTTP 中间件链装配。

package server

import (
	"gitee.com/hq_project/yomi-boot/component/common/httpguard"
	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
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
	corsCfg := config.MustSection[corsplugin.BootConfig](services.Config, corsplugin.SectionKey).ToConfig()
	httpCfg := config.MustSection[routerplugin.BootConfig](services.Config, routerplugin.SectionKey).ToConfig()
	httpRecordCfg := config.MustSection[httprecordplugin.BootConfig](services.Config, httprecordplugin.SectionKey).ToConfig()
	authCfg := config.MustSection[tokenplugin.BootConfig](services.Config, tokenplugin.SectionKey).ToConfig()
	logCfg := config.MustSection[loggerplugin.BootConfig](services.Config, loggerplugin.SectionKey).ToConfig()

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
