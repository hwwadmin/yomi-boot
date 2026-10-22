package dtmplugin

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/component/cloud/dtm"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "dtm"

// BootConfig 表示 dtm 配置段的 YAML 绑定
type BootConfig struct {
	Enabled *bool  `mapstructure:"enabled"` // Enabled 是否启用 DTM，未配置默认 false
	Server  string `mapstructure:"server"`  // Server 是 DTM 服务器地址
	Timeout int    `mapstructure:"timeout"` // Timeout 是控制面请求超时秒数，未配置默认 3
}

// EnabledOrDefault 返回是否启用 DTM，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() dtm.Config {
	return dtm.Config{Server: c.Server, Timeout: c.Timeout}
}

// Validate 校验 dtm 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if strings.TrimSpace(c.Server) == "" {
		return errors.New("dtm.server 不能为空")
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 dtm 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
