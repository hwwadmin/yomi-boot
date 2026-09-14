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
	Version string `json:"version"` // Version 是业务版本；空则由源码 hash 派生
}

// EffectiveVersion 返回用于编译缓存键的版本；Version 为空时对 Source 做 sha256
func (s *Script) EffectiveVersion() string {
	if s == nil {
		return ""
	}
	if v := strings.TrimSpace(s.Version); v != "" {
		return v
	}
	sum := sha256.Sum256([]byte(s.Source))
	return hex.EncodeToString(sum[:])
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
		s.Version = s.EffectiveVersion()
	}
	return nil
}
