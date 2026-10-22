// Package redis 提供 Redis 访问配置
//
// @author zwei
package redis

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// DefaultCommandTimeoutMs 未配置 command_timeout_ms 时的默认指令执行最长等待时间（毫秒）
const DefaultCommandTimeoutMs = 3000

// Config 表示 Redis 访问配置
type Config struct {
	Addr             string // Addr 是 Redis 地址
	Password         string // Password 是 Redis 密码
	DB               int    // DB 是 Redis 数据库编号
	CommandTimeoutMs int    // CommandTimeoutMs 是单条指令执行的最长等待时间(毫秒)，未配置或 <=0 时默认 3000
}

// CommandTimeout 返回单条指令执行的最长等待时间；未配置或 <=0 时默认 3000ms
func (c Config) CommandTimeout() time.Duration {
	ms := c.CommandTimeoutMs
	if ms <= 0 {
		ms = DefaultCommandTimeoutMs
	}
	return time.Duration(ms) * time.Millisecond
}

// Validate 校验 Redis 配置语义
func (c Config) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("addr 不能为空")
	}
	if c.CommandTimeoutMs < 0 {
		return errors.New("command_timeout_ms 不能小于 0")
	}
	return nil
}
