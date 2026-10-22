package dbplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"
)

// SectionKey YAML 根 key
const SectionKey = "db"

// BootConfig 表示 db 配置段的 YAML 绑定
type BootConfig struct {
	Enabled                *bool        `mapstructure:"enabled"`                    // Enabled 是否启用数据库，未配置默认 false
	Type                   enums.DbType `mapstructure:"type"`                       // Type 是数据库类型
	Host                   string       `mapstructure:"host"`                       // Host 是数据库地址
	Port                   int          `mapstructure:"port"`                       // Port 是数据库端口
	User                   string       `mapstructure:"user"`                       // User 是数据库用户名
	Password               string       `mapstructure:"password"`                   // Password 是数据库密码
	Database               string       `mapstructure:"database"`                   // Database 是数据库名
	Params                 string       `mapstructure:"params"`                     // Params 是 DSN 追加参数；MySQL 未配置 parseTime/loc 时自动补 parseTime=true&loc=Local
	MaxOpenConns           int          `mapstructure:"max_open_conns"`             // MaxOpenConns 是最大连接数
	MaxIdleConns           int          `mapstructure:"max_idle_conns"`             // MaxIdleConns 是最大空闲连接数
	MinIdleConns           int          `mapstructure:"min_idle_conns"`             // MinIdleConns 是最小空闲连接数，0不预热不保活，须<=max_idle_conns
	ConnMaxLifetimeMinutes int          `mapstructure:"conn_max_lifetime_minutes"`  // ConnMaxLifetimeMinutes 是连接最大存活分钟数
	ConnMaxIdleTimeMinutes int          `mapstructure:"conn_max_idle_time_minutes"` // ConnMaxIdleTimeMinutes 是空闲连接最大存活分钟数
	SlowThresholdMs        *int         `mapstructure:"slow_threshold_ms"`          // SlowThresholdMs 慢 SQL 阈值(毫秒)，未配置默认500，0关闭
}

// EnabledOrDefault 返回是否启用数据库，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return c.Enabled != nil && *c.Enabled
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() db.Config {
	return db.Config{
		Type:                   c.Type,
		Host:                   c.Host,
		Port:                   c.Port,
		User:                   c.User,
		Password:               c.Password,
		Database:               c.Database,
		Params:                 c.Params,
		MaxOpenConns:           c.MaxOpenConns,
		MaxIdleConns:           c.MaxIdleConns,
		MinIdleConns:           c.MinIdleConns,
		ConnMaxLifetimeMinutes: c.ConnMaxLifetimeMinutes,
		ConnMaxIdleTimeMinutes: c.ConnMaxIdleTimeMinutes,
		SlowThresholdMs:        c.SlowThresholdMs,
	}
}

// Validate 校验 db 配置段语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 db 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Document) error {
		return c.Validate()
	})
}
