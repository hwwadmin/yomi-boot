// Package db 提供数据库访问配置
//
// @author zwei
package db

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/shared/enums"
)

// SectionKey YAML 根 key
const SectionKey = "db"

// DefaultSlowThresholdMs 未配置 slow_threshold_ms 时的默认慢 SQL 阈值
const DefaultSlowThresholdMs = 500

// Config 表示数据库访问配置
type Config struct {
	Enabled                *bool        `mapstructure:"enabled"`                    // Enabled 是否启用数据库，未配置默认 false
	Type                   enums.DbType `mapstructure:"type"`                       // Type 是数据库类型
	Host                   string       `mapstructure:"host"`                       // Host 是数据库地址
	Port                   int          `mapstructure:"port"`                       // Port 是数据库端口
	User                   string       `mapstructure:"user"`                       // User 是数据库用户名
	Password               string       `mapstructure:"password"`                   // Password 是数据库密码
	Database               string       `mapstructure:"database"`                   // Database 是数据库名
	Params                 string       `mapstructure:"params"`                     // Params 是 DSN 追加参数
	MaxOpenConns           int          `mapstructure:"max_open_conns"`             // MaxOpenConns 是最大连接数
	MaxIdleConns           int          `mapstructure:"max_idle_conns"`             // MaxIdleConns 是最大空闲连接数
	MinIdleConns           int          `mapstructure:"min_idle_conns"`             // MinIdleConns 是最小空闲连接数，0不预热不保活，须<=max_idle_conns
	ConnMaxLifetimeMinutes int          `mapstructure:"conn_max_lifetime_minutes"`  // ConnMaxLifetimeMinutes 是连接最大存活分钟数
	ConnMaxIdleTimeMinutes int          `mapstructure:"conn_max_idle_time_minutes"` // ConnMaxIdleTimeMinutes 是空闲连接最大存活分钟数
	SlowThresholdMs        *int         `mapstructure:"slow_threshold_ms"`          // SlowThresholdMs 慢 SQL 阈值(毫秒)，未配置默认500，0关闭
}

// EnabledOrDefault 返回是否启用数据库，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// SlowThreshold 返回 GORM 慢 SQL 阈值；未配置默认 500ms，配置为 0 时关闭慢 SQL 日志
func (c Config) SlowThreshold() time.Duration {
	if c.SlowThresholdMs == nil {
		return time.Duration(DefaultSlowThresholdMs) * time.Millisecond
	}
	if *c.SlowThresholdMs == 0 {
		return 0
	}
	return time.Duration(*c.SlowThresholdMs) * time.Millisecond
}

// Validate 校验数据库配置语义（仅 enabled=true 时校验）
func (c Config) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("db.host 不能为空")
	}
	if c.Port <= 0 {
		return errors.New("db.port 必须大于 0")
	}
	if strings.TrimSpace(c.User) == "" {
		return errors.New("db.user 不能为空")
	}
	if strings.TrimSpace(c.Database) == "" {
		return errors.New("db.database 不能为空")
	}
	if c.Type != enums.MySQL && c.Type != enums.Postgres {
		return errors.Newf("db.type 不支持: %s", c.Type)
	}
	if c.MaxOpenConns <= 0 {
		return errors.New("db.max_open_conns 必须大于 0")
	}
	if c.MaxIdleConns <= 0 {
		return errors.New("db.max_idle_conns 必须大于 0")
	}
	if c.MinIdleConns < 0 {
		return errors.New("db.min_idle_conns 不能小于 0")
	}
	if c.MinIdleConns > c.MaxIdleConns {
		return errors.New("db.min_idle_conns 不能大于 db.max_idle_conns")
	}
	if c.MinIdleConns > c.MaxOpenConns {
		return errors.New("db.min_idle_conns 不能大于 db.max_open_conns")
	}
	if c.ConnMaxLifetimeMinutes <= 0 {
		return errors.New("db.conn_max_lifetime_minutes 必须大于 0")
	}
	if c.ConnMaxIdleTimeMinutes <= 0 {
		return errors.New("db.conn_max_idle_time_minutes 必须大于 0")
	}
	if c.SlowThresholdMs != nil && *c.SlowThresholdMs < 0 {
		return errors.New("db.slow_threshold_ms 不能小于 0")
	}
	return nil
}

// ConfigSection 返回 db 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
