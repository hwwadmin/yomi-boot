package appctxplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "app"

// BootConfig 表示 app 配置段的 YAML 绑定
type BootConfig struct {
	Name     string `mapstructure:"name"`     // Name 是服务名称
	Env      string `mapstructure:"env"`      // Env 是当前运行环境
	Version  string `mapstructure:"version"`  // Version 是当前版本号
	Timezone string `mapstructure:"timezone"` // Timezone 是进程时区，启动时锁定到 time.Local，默认 UTC
}

// ToConfig 转换为运行时配置并填充默认值
func (c BootConfig) ToConfig() appctx.Config {
	cfg := appctx.Config{
		Name:     c.Name,
		Env:      c.Env,
		Version:  c.Version,
		Timezone: c.Timezone,
	}
	cfg.Normalize()
	return cfg
}

// Validate 校验 app 配置段语义
func (c BootConfig) Validate() error {
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 app 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
