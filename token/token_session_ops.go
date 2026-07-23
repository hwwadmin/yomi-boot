// Package token 提供令牌会话上下文操作能力
//
// @author zwei
package token

import (
	"context"
	"strings"
	"yt-boot/errors"
)

// contextKey 是令牌会话在上下文中的键
type contextKey struct{}

// tokenValueContextKey 是原始令牌值在上下文中的键
type tokenValueContextKey struct{}

// WithSession 将鉴权会话写入请求上下文
func WithSession(ctx context.Context, session *TokenSession) context.Context {
	return context.WithValue(ctx, contextKey{}, session)
}

// SessionFromContext 从请求上下文读取鉴权会话
func SessionFromContext(ctx context.Context) (*TokenSession, bool) {
	session, ok := ctx.Value(contextKey{}).(*TokenSession)
	return session, ok
}

// OperatorID 从令牌会话中返回当前操作者用户 ID
func OperatorID(ctx context.Context) (int64, error) {
	session, ok := SessionFromContext(ctx)
	if !ok || session == nil || session.UserID <= 0 {
		return 0, errors.Sentinel(ErrTokenInvalid)
	}

	return session.UserID, nil
}

// WithTokenValue 将当前请求对应的原始令牌写入上下文
func WithTokenValue(ctx context.Context, tokenValue string) context.Context {
	return context.WithValue(ctx, tokenValueContextKey{}, strings.TrimSpace(tokenValue))
}

// TokenValueFromContext 从请求上下文读取原始令牌值
func TokenValueFromContext(ctx context.Context) (string, bool) {
	tokenValue, ok := ctx.Value(tokenValueContextKey{}).(string)
	if !ok || strings.TrimSpace(tokenValue) == "" {
		return "", false
	}

	return tokenValue, true
}

// TokenContextMapFromContext 从请求上下文读取令牌自定义上下文参数集合的只读快照
func TokenContextMapFromContext(ctx context.Context) map[string]any {
	session, ok := SessionFromContext(ctx)
	if !ok || session == nil {
		return nil
	}

	return session.ContextMap()
}

// TokenContextValueFromContext 从请求上下文读取令牌自定义上下文字段
func TokenContextValueFromContext(ctx context.Context, key string) (any, bool) {
	session, ok := SessionFromContext(ctx)
	if !ok || session == nil {
		return nil, false
	}

	return session.ContextValue(key)
}

// SetTokenContextValue 向请求上下文中的令牌会话写入自定义上下文字段
func SetTokenContextValue(ctx context.Context, key string, value any) bool {
	session, ok := SessionFromContext(ctx)
	if !ok || session == nil {
		return false
	}

	session.SetContextValue(key, value)
	return true
}

// DeleteTokenContextValue 删除请求上下文中的令牌自定义上下文字段
func DeleteTokenContextValue(ctx context.Context, key string) bool {
	session, ok := SessionFromContext(ctx)
	if !ok || session == nil {
		return false
	}

	session.DeleteContextValue(key)
	return true
}
