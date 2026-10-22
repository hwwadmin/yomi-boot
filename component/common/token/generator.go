// Package token 提供令牌基础设施骨架
//
// @author zwei
package token

import "github.com/google/uuid"

// Generator 表示令牌生成器
type Generator interface {
	// NewToken 生成新的令牌字符串
	NewToken() (string, error)
}

// UUIDGenerator 表示基于 UUID 的令牌生成器
type UUIDGenerator struct{}

var _ Generator = (*UUIDGenerator)(nil)

// NewUUIDGenerator 创建 UUID 令牌生成器
func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{}
}

// NewToken 生成新的 UUID 令牌
func (g *UUIDGenerator) NewToken() (string, error) {
	return uuid.NewString(), nil
}
