// Package router Fail Fast 初始化
//
// @author zwei
package router

import (
	"gitee.com/hq_project/yomi-boot/logger"

	"github.com/gin-gonic/gin"
)

// MustNewRouter 创建 gin Engine，失败时 log + panic
func MustNewRouter(routerName string, registrars []Registrar, middlewares []gin.HandlerFunc, trustedProxies []string) *gin.Engine {
	engine, err := NewRouter(routerName, registrars, middlewares, trustedProxies)
	if err != nil {
		logger.ErrorWithoutCtx("路由初始化失败", "error", err)
		panic(err)
	}
	return engine
}
