package loggerplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "log"

// RotationBootConfig 文件滚动与保留配置的 YAML 绑定
type RotationBootConfig struct {
	MaxSizeMB     int   `mapstructure:"max_size_mb"`    // 单文件大小上限（MB）
	RetentionDays int   `mapstructure:"retention_days"` // 历史保留天数
	MaxBackups    int   `mapstructure:"max_backups"`    // 历史文件个数上限
	MaxTotalMB    int   `mapstructure:"max_total_mb"`   // 单类型目录总容量上限（MB）
	Compress      *bool `mapstructure:"compress"`       // 历史是否 gzip；nil 默认 true
}

// AccessBootConfig 文件 access 中间件配置的 YAML 绑定
type AccessBootConfig struct {
	Enabled       *bool    `mapstructure:"enabled"`         // nil 默认开启
	SkipPaths     []string `mapstructure:"skip_paths"`      // 不记录的路径前缀/精确路径
	MaxBodyBytes  int      `mapstructure:"max_body_bytes"`  // body 采集上限
	MaxFieldBytes int      `mapstructure:"max_field_bytes"` // 单字段（如 header 值）上限
	RecordBody    bool     `mapstructure:"record_body"`     // 是否采集 body
	ToConsole     bool     `mapstructure:"to_console"`      // access 是否同时打控制台
	SampleRate    float64  `mapstructure:"sample_rate"`     // 默认 1 全量；(0,1) 仅约束成功请求；>=400 必采
}

// BootConfig 表示 log 配置段的 YAML 绑定
type BootConfig struct {
	Dir            string             `mapstructure:"dir"`             // 日志根目录（其下 access/info/error/debug）
	Level          string             `mapstructure:"level"`           // 文件最低级别
	ConsoleLevel   string             `mapstructure:"console_level"`   // 控制台最低级别
	Fields         map[string]string  `mapstructure:"fields"`          // 静态标签，自动注入每条日志
	SensitiveMatch string             `mapstructure:"sensitive_match"` // contains（默认）或 exact
	SensitiveKeys  []string           `mapstructure:"sensitive_keys"`  // 敏感字段名；空=不脱敏
	Rotation       RotationBootConfig `mapstructure:"rotation"`        // 滚动与保留（必填）
	Access         AccessBootConfig   `mapstructure:"access"`          // 文件 access 中间件
}

// ToConfig 转换为运行时配置并填充默认值
func (c BootConfig) ToConfig() logger.Config {
	cfg := logger.Config{
		Dir:            c.Dir,
		Level:          c.Level,
		ConsoleLevel:   c.ConsoleLevel,
		Fields:         c.Fields,
		SensitiveMatch: c.SensitiveMatch,
		SensitiveKeys:  c.SensitiveKeys,
		Rotation: logger.RotationConfig{
			MaxSizeMB:     c.Rotation.MaxSizeMB,
			RetentionDays: c.Rotation.RetentionDays,
			MaxBackups:    c.Rotation.MaxBackups,
			MaxTotalMB:    c.Rotation.MaxTotalMB,
			Compress:      c.Rotation.Compress,
		},
		Access: logger.AccessConfig{
			Enabled:       c.Access.Enabled,
			SkipPaths:     c.Access.SkipPaths,
			MaxBodyBytes:  c.Access.MaxBodyBytes,
			MaxFieldBytes: c.Access.MaxFieldBytes,
			RecordBody:    c.Access.RecordBody,
			ToConsole:     c.Access.ToConsole,
			SampleRate:    c.Access.SampleRate,
		},
	}
	_ = cfg.Normalize()
	return cfg
}

// Validate 校验 log 配置段语义
func (c BootConfig) Validate() error {
	cfg := c.ToConfig()
	if err := cfg.Normalize(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 log 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Config) error {
		return c.Validate()
	})
}
