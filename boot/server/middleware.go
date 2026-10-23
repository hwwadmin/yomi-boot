// HTTP 中间件链装配。

package server

import (
	"gitee.com/hq_project/yomi-boot/component/common/httprecord"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/cors"
	"gitee.com/hq_project/yomi-boot/component/core/httpguard"
	"gitee.com/hq_project/yomi-boot/component/core/traceid"
	"gitee.com/hq_project/yomi-boot/plugin"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	corsplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/cors"
	httprecordplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/httprecord"
	loggerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/logger"
	routerplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/router"
	tokenplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/token"

	"github.com/gin-gonic/gin"
)

// DefaultMiddlewares 返回通用 HTTP 中间件链
func DefaultMiddlewares(slot *plugin.PluginSlot) []gin.HandlerFunc {
	corsCfg := config.MustSection[corsplugin.BootConfig](slot.Config, corsplugin.SectionKey).ToConfig()
	httpCfg := config.MustSection[routerplugin.BootConfig](slot.Config, routerplugin.SectionKey).ToConfig()
	httpRecordCfg := config.MustSection[httprecordplugin.BootConfig](slot.Config, httprecordplugin.SectionKey).ToConfig()
	tokenCfg := config.MustSection[tokenplugin.BootConfig](slot.Config, tokenplugin.SectionKey).ToConfig()
	logCfg := config.MustSection[loggerplugin.BootConfig](slot.Config, loggerplugin.SectionKey).ToConfig()

	return []gin.HandlerFunc{
		cors.Middleware(corsCfg),
		traceid.Middleware(),
		httprecord.Middleware(httprecordplugin.From(slot).Service(), httpRecordCfg, appctxplugin.From(slot).AppCtx()),
		logger.Middleware(logCfg.Access),
		httpguard.Middleware(httpCfg),
		gin.Recovery(),
		token.Middleware(tokenplugin.From(slot).Service(), tokenCfg),
	}
}
