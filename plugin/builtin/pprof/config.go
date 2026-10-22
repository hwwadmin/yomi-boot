package pprofplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/pprof"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "pprof"

// BootConfig 表示 pprof 配置段的 YAML 绑定
type BootConfig struct {
	Enabled *bool  `mapstructure:"enabled"` // Enabled 是否启用 pprof，未配置默认 false
	Addr    string `mapstructure:"addr"`    // Addr 是监听地址，如 :16080
}

// EnabledOrDefault 返回是否启用 pprof，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() pprof.Config {
	return pprof.Config{Addr: c.Addr}
}

// Validate 校验 pprof 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 pprof 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
