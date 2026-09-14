// Package golua 脚本快照模型
//
// @author zwei
package golua

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Script 表示一条脚本不可变快照（缓存与执行共用）
type Script struct {
	ID      string `json:"id"`      // ID 是业务脚本标识
	Source  string `json:"source"`  // Source 是 Lua 源码
	Version string `json:"version"` // Version 是业务版本语义；空则 normalize 时写入源码 hash
}

// ContentHash 返回源码 sha256 hex，用作进程内 Proto 缓存键（与业务 Version 无关）
func (s *Script) ContentHash() string {
	if s == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(s.Source))
	return hex.EncodeToString(sum[:])
}

// EffectiveVersion 返回业务版本；Version 为空时回落为源码 hash（供快照字段填充，非 Proto 键）
func (s *Script) EffectiveVersion() string {
	if s == nil {
		return ""
	}
	if v := strings.TrimSpace(s.Version); v != "" {
		return v
	}
	return s.ContentHash()
}

// normalize 校验并规范化脚本字段
func (s *Script) normalize() error {
	if s == nil {
		return errors.New("golua: 脚本不能为空")
	}
	s.ID = strings.TrimSpace(s.ID)
	if s.ID == "" {
		return errors.New("golua: 脚本 id 不能为空")
	}
	if strings.TrimSpace(s.Source) == "" {
		return errors.New("golua: 脚本 source 不能为空")
	}
	s.Version = strings.TrimSpace(s.Version)
	if s.Version == "" {
		s.Version = s.ContentHash()
	}
	return nil
}
