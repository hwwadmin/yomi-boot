package cors

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const defaultMaxAgeSeconds = 86400

// Middleware 返回基于配置的 CORS 中间件；未启用时直接放行
func Middleware(cfg Config) gin.HandlerFunc {
	allowMethods := strings.Join(cfg.AllowMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	allowAllOrigins := len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*"

	return func(c *gin.Context) {
		if !cfg.MiddlewareEnabledOrDefault() {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")

		// 携带凭证时不能使用 *，需回显具体 Origin
		if cfg.AllowCredentials && allowAllOrigins && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		} else if allowAllOrigins {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if isOriginAllowed(origin, cfg.AllowOrigins) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		maxAge := cfg.MaxAge
		if maxAge <= 0 {
			maxAge = defaultMaxAgeSeconds
		}
		c.Header("Access-Control-Max-Age", strconv.Itoa(maxAge))

		c.Header("Access-Control-Allow-Methods", allowMethods)
		c.Header("Access-Control-Allow-Headers", allowHeaders)

		// 预检请求直接返回 204
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// isOriginAllowed 检查请求来源是否在允许列表中
func isOriginAllowed(origin string, allowOrigins []string) bool {
	if origin == "" {
		return false
	}
	for _, o := range allowOrigins {
		if o == origin {
			return true
		}
	}
	return false
}
