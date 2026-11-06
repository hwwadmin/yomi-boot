// Package token 提供鉴权中间件能力
//
// @author zwei
package token

import (
	"path"
	"strings"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/handler"

	"github.com/gin-gonic/gin"
)

var (
	// ErrTokenRequired 缺少访问令牌
	ErrTokenRequired = errors.NewNoStack("TOKEN_REQUIRED")
	// ErrTokenValidateFailed 令牌校验失败
	ErrTokenValidateFailed = errors.NewNoStack("TOKEN_VALIDATE_FAILED")
)

// TokenMiddleware 表示令牌鉴权中间件实例
type TokenMiddleware struct {
	tokenService TokenService
	authConfig   Config
}

// Handler 返回当前中间件实例对应的 gin 中间件处理器
func (m *TokenMiddleware) Handler() gin.HandlerFunc {
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

// Middleware 返回基于 AuthConfig 注入的令牌鉴权中间件（gin.HandlerFunc）
func Middleware(tokenService TokenService, authConfig Config) gin.HandlerFunc {
	return (&TokenMiddleware{
		tokenService: tokenService,
		authConfig:   authConfig,
	}).Handler()
}

// shouldSkipValidate 判断请求路径是否需要跳过令牌校验。根路径只认 "/"，空字符串和 "." 不是根路径。
func (m *TokenMiddleware) shouldSkipValidate(requestPath string) bool {
	currentPath := cleanSkipPath(requestPath)
	for _, skipPath := range m.authConfig.SkipValidatePaths {
		if cleanSkipPath(skipPath) == currentPath {
			return true
		}
	}
	return false
}

// cleanSkipPath 去掉首尾空白并规范化路径。空字符串和 "." 保持为 "."，不收成 "/"。
func cleanSkipPath(raw string) string {
	return path.Clean(strings.TrimSpace(raw))
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
