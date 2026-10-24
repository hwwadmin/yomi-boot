// 数据源配置
//
// @author zwei

package db

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"
)

// DefaultSlowThresholdMs 未配置 slow_threshold_ms 时的默认慢 SQL 阈值
const DefaultSlowThresholdMs = 500

// Config 表示数据库访问配置
type Config struct {
	Type                   enums.DbType // Type 是数据库类型
	Host                   string       // Host 是数据库地址
	Port                   int          // Port 是数据库端口
	User                   string       // User 是数据库用户名
	Password               string       // Password 是数据库密码
	Database               string       // Database 是数据库名
	Params                 string       // Params 是 DSN 追加参数；MySQL 未配置 parseTime/loc 时自动补 parseTime=true&loc=Local
	MaxOpenConns           int          // MaxOpenConns 是最大连接数
	MaxIdleConns           int          // MaxIdleConns 是最大空闲连接数
	MinIdleConns           int          // MinIdleConns 是最小空闲连接数，0不预热不保活，须<=max_idle_conns
	ConnMaxLifetimeMinutes int          // ConnMaxLifetimeMinutes 是连接最大存活分钟数
	ConnMaxIdleTimeMinutes int          // ConnMaxIdleTimeMinutes 是空闲连接最大存活分钟数
	SlowThresholdMs        *int         // SlowThresholdMs 慢 SQL 阈值(毫秒)，未配置默认500，0关闭
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

// Validate 校验数据库配置语义
func (c Config) Validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("host 不能为空")
	}
	if c.Port <= 0 {
		return errors.New("port 必须大于 0")
	}
	if strings.TrimSpace(c.User) == "" {
		return errors.New("user 不能为空")
	}
	if strings.TrimSpace(c.Database) == "" {
		return errors.New("database 不能为空")
	}
	if c.Type != enums.MySQL && c.Type != enums.Postgres {
		return errors.Newf("type 不支持: %s", c.Type)
	}
	if c.MaxOpenConns <= 0 {
		return errors.New("max_open_conns 必须大于 0")
	}
	if c.MaxIdleConns <= 0 {
		return errors.New("max_idle_conns 必须大于 0")
	}
	if c.MinIdleConns < 0 {
		return errors.New("min_idle_conns 不能小于 0")
	}
	if c.MinIdleConns > c.MaxIdleConns {
		return errors.New("min_idle_conns 不能大于 max_idle_conns")
	}
	if c.MinIdleConns > c.MaxOpenConns {
		return errors.New("min_idle_conns 不能大于 max_open_conns")
	}
	if c.ConnMaxLifetimeMinutes <= 0 {
		return errors.New("conn_max_lifetime_minutes 必须大于 0")
	}
	if c.ConnMaxIdleTimeMinutes <= 0 {
		return errors.New("conn_max_idle_time_minutes 必须大于 0")
	}
	if c.SlowThresholdMs != nil && *c.SlowThresholdMs < 0 {
		return errors.New("slow_threshold_ms 不能小于 0")
	}
	return nil
}
