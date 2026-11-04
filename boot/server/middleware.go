// HTTP 中间件链装配。

package server

import (
	"github.com/hwwadmin/yomi-boot/component/common/httprecord"
	"github.com/hwwadmin/yomi-boot/component/common/token"
	"github.com/hwwadmin/yomi-boot/component/core/config"
	"github.com/hwwadmin/yomi-boot/component/core/cors"
	"github.com/hwwadmin/yomi-boot/component/core/httpguard"
	"github.com/hwwadmin/yomi-boot/component/core/logger"
	"github.com/hwwadmin/yomi-boot/component/core/traceid"
	"github.com/hwwadmin/yomi-boot/plugin"
	appctxplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/appctx"
	corsplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/cors"
	httprecordplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/httprecord"
	loggerplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/logger"
	maskingplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/masking"
	routerplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/router"
	tokenplugin "github.com/hwwadmin/yomi-boot/plugin/builtin/token"

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
		httprecord.Middleware(httprecordplugin.From(slot).Service(), httpRecordCfg, appctxplugin.From(slot).AppCtx(), maskingplugin.From(slot).Service()),
		logger.Middleware(logCfg.Access),
		gin.Recovery(),
		httpguard.Middleware(httpCfg),
		token.Middleware(tokenplugin.From(slot).Service(), tokenCfg),
	}
}
