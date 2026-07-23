// Package token 提供令牌存储会话模型定义
//
// @author zwei
package token

import (
	"encoding/json"
	"gitee.com/hq_project/yomi-boot/shared/dto"
	"time"
)

// TokenSession 表示令牌对应的会话快照
type TokenSession struct {
	UserID          int64       `json:"user_id"`               // UserID 是当前登录用户标识
	RoleIDs         []int64     `json:"role_ids"`              // RoleIDs 是当前用户角色集合
	TenantID        *int64      `json:"tenant_id,omitempty"`   // TenantID 是租户标识
	ClientType      string      `json:"client_type,omitempty"` // ClientType 是端类型
	DeviceID        string      `json:"device_id,omitempty"`   // DeviceID 是设备标识
	LoginIP         string      `json:"login_ip,omitempty"`    // LoginIP 是登录 IP
	IssuedAt        time.Time   `json:"issued_at"`             // IssuedAt 是签发时间
	ExpireAt        time.Time   `json:"expire_at"`             // ExpireAt 是过期时间快照
	AllowMultiLogin bool        `json:"allow_multi_login"`     // AllowMultiLogin 表示是否允许并发登录
	context         dto.SafeMap // context 保存业务自定义的令牌上下文参数，值需要可被 JSON 序列化
}

// UserTokenSession 表示用户维度查询出的令牌及其会话快照
type UserTokenSession struct {
	Token   string        `json:"token"`
	Session *TokenSession `json:"session"`
}

// MarshalJSON 序列化令牌会话快照，Context 输出为独立副本
func (s *TokenSession) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}

	type tokenSessionAlias TokenSession
	return json.Marshal(&struct {
		*tokenSessionAlias
		Context map[string]any `json:"context,omitempty"`
	}{
		tokenSessionAlias: (*tokenSessionAlias)(s),
		Context:           s.context.Snapshot(),
	})
}

// UnmarshalJSON 反序列化令牌会话快照
func (s *TokenSession) UnmarshalJSON(data []byte) error {
	if s == nil {
		return nil
	}

	type tokenSessionAlias TokenSession
	aux := &struct {
		*tokenSessionAlias
		Context map[string]any `json:"context,omitempty"`
	}{
		tokenSessionAlias: (*tokenSessionAlias)(s),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	s.context.Replace(aux.Context)
	return nil
}

// ContextMap 返回自定义令牌上下文参数集合的只读快照
func (s *TokenSession) ContextMap() map[string]any {
	if s == nil {
		return nil
	}

	return s.context.Snapshot()
}

// SetContextValue 写入自定义令牌上下文字段
func (s *TokenSession) SetContextValue(key string, value any) {
	if s == nil || key == "" {
		return
	}

	s.context.Set(key, value)
}

// ContextValue 读取自定义令牌上下文字段
func (s *TokenSession) ContextValue(key string) (any, bool) {
	if s == nil || key == "" {
		return nil, false
	}

	return s.context.Get(key)
}

// Expired 判断会话快照是否已过期
func (s *TokenSession) Expired() bool {
	if s == nil {
		return true
	}
	if s.ExpireAt.IsZero() {
		return true
	}

	return time.Now().After(s.ExpireAt)
}

// DeleteContextValue 删除自定义令牌上下文字段
func (s *TokenSession) DeleteContextValue(key string) {
	if s == nil || key == "" {
		return
	}

	s.context.Delete(key)
}
