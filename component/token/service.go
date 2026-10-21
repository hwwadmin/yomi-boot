// Package token 提供令牌签发与校验能力
//
// @author zwei
package token

import (
	"context"
	"sort"
	"strconv"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	sharederr "gitee.com/hq_project/yomi-boot/boot/shared/error"
	"gitee.com/hq_project/yomi-boot/component/distlock"
)

// ErrTokenInvalid 令牌无效或已过期
var ErrTokenInvalid = errors.NewNoStack("TOKEN_INVALID")

// TokenService 表示令牌服务能力接口
type TokenService interface {
	// Issue 签发新的访问令牌
	Issue(ctx context.Context, session *TokenSession) (string, error)
	// Validate 校验令牌并返回会话快照
	Validate(ctx context.Context, token string) (*TokenSession, error)
	// Revoke 使指定令牌立即失效
	Revoke(ctx context.Context, token string) error
	// RevokeByUserID 使指定用户的全部令牌立即失效
	RevokeByUserID(ctx context.Context, userID int64) error
}

const issueLockKeySuffix = ":lock:issue:user:"

// Service 表示令牌基础服务
type Service struct {
	generator       Generator
	store           Store
	authConfig      Config
	ttl             time.Duration
	distLockFactory distlock.DistLockFactory
}

var _ TokenService = (*Service)(nil)

// NewService 创建令牌基础服务
func NewService(
	generator Generator,
	store Store,
	authConfig Config,
	distLockFactory distlock.DistLockFactory,
) *Service {
	return &Service{
		generator:       generator,
		store:           store,
		authConfig:      authConfig,
		ttl:             authConfig.TokenTTL(),
		distLockFactory: distLockFactory,
	}
}

// Issue 签发新的访问令牌
func (s *Service) Issue(ctx context.Context, session *TokenSession) (string, error) {
	if session == nil {
		session = &TokenSession{}
	}

	now := time.Now()
	if session.IssuedAt.IsZero() {
		session.IssuedAt = now
	}
	if session.ExpireAt.IsZero() {
		session.ExpireAt = session.IssuedAt.Add(s.ttl)
	}
	session.AllowMultiLogin = s.authConfig.AllowMultiLogin
	if err := validateSessionSnapshot(session); err != nil {
		return "", errors.Wrap(err, "令牌会话不合法")
	}
	if s.authConfig.AllowMultiLogin {
		return s.issueToken(ctx, session)
	}
	if s.distLockFactory == nil {
		return "", errors.New("单点登录模式需要分布式锁工厂")
	}

	lock := s.distLockFactory.GetLock(
		issueLockKey(s.authConfig.StorePrefix, session.UserID),
		distlock.WithWatchdog(false),
		distlock.WithTTL(30*time.Second),
	)

	var tokenValue string
	err := distlock.RunWithLock(ctx, lock, func(runCtx context.Context) error {
		var innerErr error
		tokenValue, innerErr = s.issueToken(runCtx, session)
		return innerErr
	})
	if err != nil {
		return "", err
	}

	return tokenValue, nil
}

// issueToken 删除旧会话、生成新令牌并持久化；单点登录场景下须由调用方持 userID 锁
func (s *Service) issueToken(ctx context.Context, session *TokenSession) (string, error) {
	if !s.authConfig.AllowMultiLogin {
		if err := s.store.DeleteByUserID(ctx, s.authConfig.StorePrefix, session.UserID); err != nil {
			return "", err
		}
	}

	tokenValue, err := s.generator.NewToken()
	if err != nil {
		return "", err
	}
	if err := s.store.Save(ctx, s.authConfig.StorePrefix, tokenValue, session, s.ttl); err != nil {
		return "", err
	}

	return tokenValue, nil
}

// issueLockKey 生成单点登录签发流程的 userID 维度分布式锁 Key
func issueLockKey(storePrefix string, userID int64) string {
	return storePrefix + issueLockKeySuffix + strconv.FormatInt(userID, 10)
}

// Validate 校验令牌并返回会话快照
func (s *Service) Validate(ctx context.Context, token string) (*TokenSession, error) {
	if token == "" {
		return nil, errors.Sentinel(ErrTokenInvalid)
	}

	session, err := s.store.Load(ctx, s.authConfig.StorePrefix, token)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.Sentinel(ErrTokenInvalid)
	}
	if err := validateSessionSnapshot(session); err != nil {
		return nil, errors.Sentinel(ErrTokenInvalid)
	}
	if session.Expired() {
		_ = s.store.Delete(ctx, s.authConfig.StorePrefix, token)
		return nil, errors.Sentinel(ErrTokenInvalid)
	}

	return session, nil
}

// ListByUserID 根据用户ID读取全部有效令牌及其会话快照
func (s *Service) ListByUserID(ctx context.Context, userID int64) ([]*UserTokenSession, error) {
	if userID <= 0 {
		return nil, errors.Sentinel(sharederr.ErrInvalidInput)
	}

	tokens, err := s.store.ListByUserID(ctx, s.authConfig.StorePrefix, userID)
	if err != nil {
		return nil, err
	}

	sort.Slice(tokens, func(i int, j int) bool {
		leftIssuedAt := time.Time{}
		rightIssuedAt := time.Time{}
		if tokens[i] != nil && tokens[i].Session != nil {
			leftIssuedAt = tokens[i].Session.IssuedAt
		}
		if tokens[j] != nil && tokens[j].Session != nil {
			rightIssuedAt = tokens[j].Session.IssuedAt
		}
		if leftIssuedAt.Equal(rightIssuedAt) {
			return tokens[i].Token < tokens[j].Token
		}

		return leftIssuedAt.After(rightIssuedAt)
	})

	return tokens, nil
}

// Revoke 使指定令牌立即失效
func (s *Service) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return errors.Sentinel(ErrTokenInvalid)
	}

	return s.store.Delete(ctx, s.authConfig.StorePrefix, token)
}

// RevokeByUserID 使指定用户的全部令牌立即失效
func (s *Service) RevokeByUserID(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return errors.Sentinel(sharederr.ErrInvalidInput)
	}

	return s.store.DeleteByUserID(ctx, s.authConfig.StorePrefix, userID)
}

// validateSessionSnapshot 校验令牌会话快照的关键字段
func validateSessionSnapshot(session *TokenSession) error {
	if session == nil {
		return errors.New("令牌会话不能为空")
	}
	if session.UserID <= 0 {
		return errors.New("用户ID不能为空")
	}
	if session.IssuedAt.IsZero() {
		return errors.New("签发时间不能为空")
	}

	return nil
}
