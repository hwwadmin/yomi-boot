// Package golua 脚本快照模型
//
// @author zwei
package golua

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/bc/hash"
	"gitee.com/hq_project/yomi-boot/errors"
)

// Script 脚本快照
type Script struct {
	ID      string `json:"id"`      // 业务脚本标识
	Source  string `json:"source"`  // Lua 源码
	Version string `json:"version"` // 业务版本；空则填入源码 hash
}

// ContentHash 返回源码 sha256 hex
func (s *Script) ContentHash() string {
	if s == nil {
		return ""
	}
	return hash.SHA256Hex([]byte(s.Source))
}

// EffectiveVersion 返回业务版本；为空时使用源码 hash
func (s *Script) EffectiveVersion() string {
	if s == nil {
		return ""
	}
	if v := strings.TrimSpace(s.Version); v != "" {
		return v
	}
	return s.ContentHash()
}

// normalize 校验并规范化字段
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
