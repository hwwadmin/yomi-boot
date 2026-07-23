// Package pprof 提供 pprof 性能剖析配置
//
// @author zwei
package pprof

import (
	"net"
	"strings"

	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/errors"
)

// SectionKey YAML 根 key
const SectionKey = "pprof"

// Config 表示 pprof 独立 debug 端口配置
type Config struct {
	Enabled *bool  `mapstructure:"enabled"` // Enabled 是否启用 pprof，未配置默认 false
	Addr    string `mapstructure:"addr"`    // Addr 是监听地址，如 :16080
}

// EnabledOrDefault 返回是否启用 pprof，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	if c.Enabled == nil {
		return false
	}
	return *c.Enabled
}

// ListenAddr 返回规范化后的监听地址
func (c Config) ListenAddr() string {
	return strings.TrimSpace(c.Addr)
}

// Validate 校验 pprof 配置语义（仅 enabled=true 时校验）
func (c Config) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	addr := c.ListenAddr()
	if addr == "" {
		return errors.New("pprof.addr 不能为空")
	}
	if _, err := net.ResolveTCPAddr("tcp", addr); err != nil {
		return errors.Wrap(err, "pprof.addr 不合法")
	}
	return nil
}

// ConfigSection 返回 pprof 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
