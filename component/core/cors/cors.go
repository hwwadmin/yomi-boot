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

		// 来源列表里要不要写 *，由业务库的配置决定，本包不替它拒绝。
		// 浏览器不允许 Access-Control-Allow-Origin: * 和凭证同时出现，
		// 所以配置为 * 且允许凭证时，回显请求里的 Origin。
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

		// 浏览器预检才直接 204，CORS 头已写在上面。其它 OPTIONS 交给后面的路由。
		if isPreflight(c.Request) {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// isPreflight 判断是不是 CORS 预检：OPTIONS，且同时带 Origin 和 Access-Control-Request-Method
func isPreflight(r *http.Request) bool {
	if r == nil || r.Method != http.MethodOptions {
		return false
	}
	if strings.TrimSpace(r.Header.Get("Origin")) == "" {
		return false
	}
	return strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")) != ""
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
