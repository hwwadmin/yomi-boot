// Package ginutil Gin 框架相关的 HTTP 工具函数
//
// @author zwei
package ginutil

import "github.com/gin-gonic/gin"

// ClientIP 从 Gin 上下文获取客户端 IP，依赖 engine.SetTrustedProxies 配置
func ClientIP(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.ClientIP()
}
