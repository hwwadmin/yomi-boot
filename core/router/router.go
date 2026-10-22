// Package router 提供 Gin 路由引擎创建与路由注册能力
//
// @author zwei
package router

import (
	"gitee.com/hq_project/yomi-boot/core/errors"

	"github.com/gin-gonic/gin"
)

// Registrar 表示可向 Gin RouterGroup 注册自身路由的处理器
type Registrar interface {
	// Routes 将当前处理器的路由注册到指定 RouterGroup
	Routes(router *gin.RouterGroup)
}

// NewRouter 创建 gin Engine，先注入中间件再注册路由，确保中间件对全部路由生效
func NewRouter(routerName string, registrars []Registrar, middlewares []gin.HandlerFunc, trustedProxies []string) (*gin.Engine, error) {
	engine := gin.New()
	if err := engine.SetTrustedProxies(trustedProxies); err != nil {
		return nil, errors.Wrap(err, "配置可信代理失败")
	}

	// 注册中间件
	for _, mw := range middlewares {
		engine.Use(mw)
	}

	// 注册路由
	group := engine.Group(routerName)
	for _, r := range registrars {
		if r == nil {
			continue
		}
		r.Routes(group)
	}

	return engine, nil
}
