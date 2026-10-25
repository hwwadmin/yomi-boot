// Package token 提供令牌存储接口定义
//
// @author zwei
package token

import (
	"context"
	"time"
)

// Store 表示令牌会话存储接口
type Store interface {
	// Save 保存令牌与会话快照
	Save(ctx context.Context, storePrefix string, token string, session *TokenSession, ttl time.Duration) error
	// Load 读取令牌对应的会话快照
	Load(ctx context.Context, storePrefix string, token string) (*TokenSession, error)
	// ListByUserID 读取指定用户的全部令牌会话
	ListByUserID(ctx context.Context, storePrefix string, userID int64) ([]*UserTokenSession, error)
	// Delete 删除令牌会话
	Delete(ctx context.Context, storePrefix string, token string) error
	// DeleteByTokenHash 删除指定存储哈希对应的令牌会话；tokenHash 为 ListByUserID 返回的存储哈希值
	DeleteByTokenHash(ctx context.Context, storePrefix string, tokenHash string) error
	// DeleteByUserID 删除指定用户的全部令牌会话
	DeleteByUserID(ctx context.Context, storePrefix string, userID int64) error
}
