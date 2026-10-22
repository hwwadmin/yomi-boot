package idempotencyplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/idempotency"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "idempotency"

// BootConfig 表示 idempotency 配置段的 YAML 绑定
type BootConfig struct {
	StaleCleanerEnabled *bool `mapstructure:"stale_cleaner_enabled"` // StaleCleanerEnabled 是否启动 Processing 超时清理，默认 false
	ScanIntervalMinutes int   `mapstructure:"scan_interval_minutes"` // ScanIntervalMinutes 是清理任务扫描间隔（分钟）
	StaleTimeoutMinutes int   `mapstructure:"stale_timeout_minutes"` // StaleTimeoutMinutes 是 Processing 超时阈值（分钟）
	BatchSize           int   `mapstructure:"batch_size"`            // BatchSize 是单轮最多清理条数
}

// EnabledOrDefault 返回是否启用 Processing 超时清理，默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.StaleCleanerEnabled != nil && *c.StaleCleanerEnabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() idempotency.Config {
	return idempotency.Config{
		ScanIntervalMinutes: c.ScanIntervalMinutes,
		StaleTimeoutMinutes: c.StaleTimeoutMinutes,
		BatchSize:           c.BatchSize,
	}
}

// Validate 校验 idempotency 配置段语义
func (c BootConfig) Validate() error {
	if c.ScanIntervalMinutes < 0 {
		return errors.New("idempotency.scan_interval_minutes 不能小于 0")
	}
	if c.StaleTimeoutMinutes < 0 {
		return errors.New("idempotency.stale_timeout_minutes 不能小于 0")
	}
	if c.BatchSize < 0 {
		return errors.New("idempotency.batch_size 不能小于 0")
	}
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 idempotency 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
