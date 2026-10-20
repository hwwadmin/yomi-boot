// Package token 提供基于 Redis 的令牌会话存储实现
//
// @author zwei
package token

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/bc/hash"
	"gitee.com/hq_project/yomi-boot/errors"

	yomiredis "gitee.com/hq_project/yomi-boot/redis"
)

var _ Store = (*RedisStore)(nil)

// luaSaveTokenSession 原子写入用户令牌索引与会话快照
// KEYS[1] = 令牌会话键，KEYS[2] = 用户令牌索引键
// ARGV[1] = 令牌值，ARGV[2] = 会话 JSON，ARGV[3] = 过期时间（毫秒，0 表示不过期）
const luaSaveTokenSession = `
redis.call('SADD', KEYS[2], ARGV[1])
if tonumber(ARGV[3]) > 0 then
    redis.call('SET', KEYS[1], ARGV[2], 'PX', tonumber(ARGV[3]))
    local indexTTL = redis.call('PTTL', KEYS[2])
    if indexTTL < tonumber(ARGV[3]) then
        redis.call('PEXPIRE', KEYS[2], tonumber(ARGV[3]))
    end
else
    redis.call('SET', KEYS[1], ARGV[2])
end
return 1
`

// luaDeleteTokenSession 原子删除令牌会话及其在用户索引中的引用
// KEYS[1] = 令牌会话键，ARGV[1] = 令牌值，ARGV[2] = 令牌存储前缀
const luaDeleteTokenSession = `
local payload = redis.call('GET', KEYS[1])
local userID = nil
if payload then
    local session = cjson.decode(payload)
    userID = tonumber(session.user_id)
end
redis.call('DEL', KEYS[1])
if userID and userID > 0 then
    local indexKey = ARGV[2] .. ':user:' .. tostring(userID) .. ':tokens'
    redis.call('SREM', indexKey, ARGV[1])
end
return 1
`

// luaDeleteByUserID 原子删除指定用户的全部令牌会话与索引
// KEYS[1] = 用户令牌索引键，ARGV[1] = 令牌存储前缀
const luaDeleteByUserID = `
local tokens = redis.call('SMEMBERS', KEYS[1])
local prefix = ARGV[1]
for i = 1, #tokens do
    redis.call('DEL', prefix .. ':' .. tokens[i])
end
redis.call('DEL', KEYS[1])
return #tokens
`

// luaRemoveStaleTokens 原子移除索引中的过期令牌，Set 为空时再删除索引键
// KEYS[1] = 用户令牌索引键，ARGV[1..n] = 待移除的过期令牌
const luaRemoveStaleTokens = `
for i = 1, #ARGV do
    redis.call('SREM', KEYS[1], ARGV[i])
end
if redis.call('SCARD', KEYS[1]) == 0 then
    redis.call('DEL', KEYS[1])
end
return 1
`

// RedisStore 表示基于 Redis String 的令牌会话存储
type RedisStore struct {
	redisService yomiredis.RedisService // redisService 是 Redis 读写服务
}

// NewRedisStore 创建基于 Redis 的令牌会话存储
func NewRedisStore(redisService yomiredis.RedisService) *RedisStore {
	return &RedisStore{redisService: redisService}
}

// Save 保存令牌与会话快照到 Redis
func (s *RedisStore) Save(ctx context.Context, storePrefix string, token string, session *TokenSession, ttl time.Duration) error {
	if session == nil {
		return errors.New("令牌会话不能为空")
	}

	key, err := s.tokenKey(storePrefix, token)
	if err != nil {
		return err
	}
	userTokensKey, err := s.userTokensKey(storePrefix, session.UserID)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(session)
	if err != nil {
		return errors.Wrap(err, "序列化令牌会话失败")
	}

	tokenValue := strings.TrimSpace(token)
	storageToken := s.hashToken(tokenValue)
	if _, err := s.redisService.OpsScript4Eval(
		ctx,
		luaSaveTokenSession,
		[]string{key, userTokensKey},
		storageToken,
		string(payload),
		ttl.Milliseconds(),
	); err != nil {
		return errors.Wrap(err, "保存令牌会话失败")
	}

	return nil
}

// Load 从 Redis 读取令牌对应的会话快照
func (s *RedisStore) Load(ctx context.Context, storePrefix string, token string) (*TokenSession, error) {
	key, err := s.tokenKey(storePrefix, token)
	if err != nil {
		return nil, err
	}

	return s.loadSessionAtKey(ctx, key)
}

// loadSessionAtKey 从 Redis 指定键读取令牌会话快照
func (s *RedisStore) loadSessionAtKey(ctx context.Context, key string) (*TokenSession, error) {
	payload, exists, err := s.redisService.OpsStr4Get(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "读取令牌会话失败")
	}
	if !exists {
		return nil, nil
	}

	session := &TokenSession{}
	if err := json.Unmarshal([]byte(payload), session); err != nil {
		return nil, errors.Wrap(err, "解析令牌会话失败")
	}

	return session, nil
}

// ListByUserID 从 Redis 读取指定用户的全部有效令牌会话
func (s *RedisStore) ListByUserID(ctx context.Context, storePrefix string, userID int64) ([]*UserTokenSession, error) {
	indexKey, err := s.userTokensKey(storePrefix, userID)
	if err != nil {
		return nil, err
	}

	tokens, err := s.redisService.OpsSet4Members(ctx, indexKey)
	if err != nil {
		return nil, errors.Wrap(err, "读取用户令牌索引失败")
	}

	result := make([]*UserTokenSession, 0, len(tokens))
	staleTokens := make([]string, 0)
	for _, storageToken := range tokens {
		key, err := s.storageKey(storePrefix, storageToken)
		if err != nil {
			return nil, err
		}

		session, err := s.loadSessionAtKey(ctx, key)
		if err != nil {
			return nil, err
		}
		if session == nil {
			staleTokens = append(staleTokens, storageToken)
			continue
		}
		if session.Expired() || validateSessionSnapshot(session) != nil || session.UserID != userID {
			staleTokens = append(staleTokens, storageToken)
			continue
		}

		result = append(result, &UserTokenSession{
			Token:   storageToken,
			Session: session,
		})
	}

	if len(staleTokens) > 0 {
		args := make([]any, len(staleTokens))
		for i, token := range staleTokens {
			args[i] = token
		}
		if _, err := s.redisService.OpsScript4Eval(ctx, luaRemoveStaleTokens, []string{indexKey}, args...); err != nil {
			return nil, errors.Wrap(err, "清理用户令牌索引失败")
		}
	}

	return result, nil
}

// Delete 删除 Redis 中的令牌会话
func (s *RedisStore) Delete(ctx context.Context, storePrefix string, token string) error {
	key, err := s.tokenKey(storePrefix, token)
	if err != nil {
		return err
	}
	prefix, err := s.storePrefixValue(storePrefix)
	if err != nil {
		return err
	}

	tokenValue := strings.TrimSpace(token)
	storageToken := s.hashToken(tokenValue)
	if _, err := s.redisService.OpsScript4Eval(
		ctx,
		luaDeleteTokenSession,
		[]string{key},
		storageToken,
		prefix,
	); err != nil {
		return errors.Wrap(err, "删除令牌会话失败")
	}

	return nil
}

// DeleteByUserID 删除 Redis 中指定用户的全部令牌会话
func (s *RedisStore) DeleteByUserID(ctx context.Context, storePrefix string, userID int64) error {
	indexKey, err := s.userTokensKey(storePrefix, userID)
	if err != nil {
		return err
	}

	prefix, err := s.storePrefixValue(storePrefix)
	if err != nil {
		return err
	}

	if _, err := s.redisService.OpsScript4Eval(ctx, luaDeleteByUserID, []string{indexKey}, prefix); err != nil {
		return errors.Wrap(err, "删除用户令牌会话失败")
	}

	return nil
}

// tokenKey 组装令牌主会话存储键（原始令牌会先哈希）
func (s *RedisStore) tokenKey(storePrefix string, token string) (string, error) {
	return s.storageKey(storePrefix, s.hashToken(token))
}

// storageKey 组装令牌存储键，storageToken 为已哈希的令牌后缀
func (s *RedisStore) storageKey(storePrefix string, storageToken string) (string, error) {
	prefix, err := s.storePrefixValue(storePrefix)
	if err != nil {
		return "", err
	}

	storageToken = strings.TrimSpace(storageToken)
	if storageToken == "" {
		return "", errors.New("令牌不能为空")
	}

	return prefix + ":" + storageToken, nil
}

// userTokensKey 组装用户令牌索引存储键
func (s *RedisStore) userTokensKey(storePrefix string, userID int64) (string, error) {
	prefix, err := s.storePrefixValue(storePrefix)
	if err != nil {
		return "", err
	}
	if userID <= 0 {
		return "", errors.New("用户ID不能为空")
	}

	return prefix + ":user:" + strconv.FormatInt(userID, 10) + ":tokens", nil
}

// storePrefixValue 规范化令牌存储前缀
func (s *RedisStore) storePrefixValue(storePrefix string) (string, error) {
	prefix := strings.Trim(strings.TrimSpace(storePrefix), ":")
	if prefix == "" {
		return "", errors.New("令牌存储前缀不能为空")
	}

	return prefix, nil
}

// hashToken 将原始令牌哈希为 Redis 存储键后缀，避免明文落库
func (s *RedisStore) hashToken(token string) string {
	token = strings.TrimSpace(token)
	return hash.SHA256Hex([]byte(token))
}
