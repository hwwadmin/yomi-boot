// Package token 提供鉴权中间件能力
//
// @author zwei
package token

import (
	"path"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/common/handler"
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/gin-gonic/gin"
)

var (
	// ErrTokenRequired 缺少访问令牌
	ErrTokenRequired = errors.NewNoStack("TOKEN_REQUIRED")
	// ErrTokenValidateFailed 令牌校验失败
	ErrTokenValidateFailed = errors.NewNoStack("TOKEN_VALIDATE_FAILED")
)

// Middleware 表示令牌鉴权中间件实例
type Middleware struct {
	tokenService TokenService
	authConfig   Config
}

// NewAuthMiddleware 创建基于 AuthConfig 注入的令牌鉴权中间件
func NewAuthMiddleware(tokenService TokenService, authConfig Config) *Middleware {
	return &Middleware{
		tokenService: tokenService,
		authConfig:   authConfig,
	}
}

// Handler 返回当前中间件实例对应的 gin 中间件处理器
func (m *Middleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !m.authConfig.MiddlewareEnabled || m.shouldSkipValidate(c.Request.URL.Path) {
			c.Next()
			return
		}

		tokenValue := extractToken(c.GetHeader("Authorization"))
		if tokenValue == "" {
			handler.WriteBusinessError(c, errors.Sentinel(ErrTokenRequired))
			c.Abort()
			return
		}

		session, err := m.tokenService.Validate(c.Request.Context(), tokenValue)
		if err != nil {
			handler.WriteBusinessErrorWithFallback(c, err, ErrTokenValidateFailed)
			c.Abort()
			return
		}

		ctx := WithSession(c.Request.Context(), session)
		ctx = WithTokenValue(ctx, tokenValue)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// AuthMiddleware 返回基于 AuthConfig 注入的令牌鉴权中间件（gin.HandlerFunc）
func AuthMiddleware(tokenService TokenService, authConfig Config) gin.HandlerFunc {
	return NewAuthMiddleware(tokenService, authConfig).Handler()
}

// shouldSkipValidate 判断请求路径是否需要跳过令牌校验
func (m *Middleware) shouldSkipValidate(requestPath string) bool {
	currentPath := path.Clean(strings.TrimSpace(requestPath))
	if currentPath == "." || currentPath == "/" {
		currentPath = "/"
	}

	for _, skipPath := range m.authConfig.SkipValidatePaths {
		normalizedPath := path.Clean(strings.TrimSpace(skipPath))
		if normalizedPath == "." || normalizedPath == "/" {
			normalizedPath = "/"
		}
		if normalizedPath == currentPath {
			return true
		}
	}

	return false
}

// extractToken 从 Authorization 请求头中提取 Bearer Token
func extractToken(authHeader string) string {
	trimmed := strings.TrimSpace(authHeader)
	if trimmed == "" {
		return ""
	}

	if len(trimmed) >= 7 && strings.EqualFold(trimmed[:7], "Bearer ") {
		return strings.TrimSpace(trimmed[7:])
	}

	return trimmed
}
