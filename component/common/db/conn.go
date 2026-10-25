// 数据源连接、连接池与生命周期
//
// @author zwei

package db

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Conn 表示已初始化的数据源，持有 GORM 句柄、数据库类型与连接池保活。
// GORM/Close 均并发安全，可在关停阶段与业务读并发调用
type Conn struct {
	db   atomic.Pointer[gorm.DB]
	typ  enums.DbType
	stop func()

	closeErr error
	once     sync.Once
}

// Open 校验配置并初始化数据源，配置非法时返回错误；ctx 约束启动预热。
// 当 min_idle_conns>0 时会预热并启动保活，返回的 Conn 在 Close 时先停保活再关池。
func Open(ctx context.Context, cfg Config) (*Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "db 配置校验失败")
	}
	return open(ctx, cfg)
}

// open 假定 cfg 已通过语义校验，仅处理连接、连接池、预热与保活
func open(ctx context.Context, cfg Config) (*Conn, error) {
	dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "生成DSN失败")
	}

	var dialector gorm.Dialector
	switch cfg.Type {
	case enums.MySQL:
		dialector = mysql.Open(dsn)
	case enums.Postgres:
		dialector = postgres.Open(dsn)
	default:
		return nil, errors.Newf("不支持的数据库类型: %s", cfg.Type)
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{
		Logger: newGormLogger(cfg),
	})
	if err != nil {
		return nil, errors.Wrap(err, "初始化数据库连接失败")
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, errors.Wrap(err, "获取底层 sql.DB 失败")
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMinutes) * time.Minute)
	idleTimeout := time.Duration(cfg.ConnMaxIdleTimeMinutes) * time.Minute
	sqlDB.SetConnMaxIdleTime(idleTimeout)

	var stop func()
	if cfg.MinIdleConns > 0 {
		warmCtx, cancel := context.WithTimeout(ctx, idleWarmTimeout)
		err = warmIdle(warmCtx, sqlDB, cfg.MinIdleConns)
		cancel()
		if err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
		keeper := startIdleKeeper(sqlDB, cfg.MinIdleConns, cfg.MaxIdleConns, maintainInterval(idleTimeout))
		if keeper != nil {
			stop = keeper.Stop
		}
	}

	logger.InfoWithoutCtx(
		"初始化数据库链接成功",
		"type", string(cfg.Type),
		"addr", cfg.Host+":"+fmt.Sprint(cfg.Port),
		"db", cfg.Database,
		"max_open_conns", cfg.MaxOpenConns,
		"max_idle_conns", cfg.MaxIdleConns,
		"min_idle_conns", cfg.MinIdleConns,
		"slow_threshold_ms", cfg.SlowThreshold().Milliseconds(),
	)

	conn := &Conn{typ: cfg.Type, stop: stop}
	conn.db.Store(gormDB)
	return conn, nil
}

// GORM 返回数据源的 GORM 句柄；未初始化或已 Close 时返回 nil
func (c *Conn) GORM() *gorm.DB {
	if c == nil {
		return nil
	}
	return c.db.Load()
}

// Type 返回数据库类型；未初始化时返回空值
func (c *Conn) Type() enums.DbType {
	if c == nil {
		return ""
	}
	return c.typ
}

// Close 停止空闲保活并关闭连接池，可重复调用；与 GORM 并发调用安全，关闭后 GORM 返回 nil
func (c *Conn) Close() error {
	if c == nil {
		return nil
	}

	c.once.Do(func() {
		if c.stop != nil {
			c.stop()
			c.stop = nil
		}
		gormDB := c.db.Load()
		if gormDB == nil {
			return
		}
		// 先置空句柄再关池：让并发读方尽早看到 nil（关闭后 GORM 返回 nil 的既定契约）
		c.db.Store(nil)
		sqlDB, dbErr := gormDB.DB()
		if dbErr != nil {
			c.closeErr = errors.Wrap(dbErr, "获取底层 sql.DB 失败")
			return
		}
		c.closeErr = sqlDB.Close()
	})
	return c.closeErr
}

// buildDSN 根据不同数据库类型返回对应格式的连接串
func buildDSN(cfg Config) (string, error) {
	switch cfg.Type {
	case enums.MySQL:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, withMySQLDefaults(cfg.Params)), nil
	case enums.Postgres:
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s %s",
			escapeDSNValue(cfg.Host), cfg.Port, escapeDSNValue(cfg.User), escapeDSNValue(cfg.Password), escapeDSNValue(cfg.Database), cfg.Params), nil
	default:
		return "", errors.Newf("不支持的数据库类型: %s", cfg.Type)
	}
}

// escapeDSNValue 转义 Postgres keyword/value 连接串的单个值：
// 不含空白字符时原样输出（引号等在未加引号的值里是普通字符）；
// 含空白字符时用单引号包裹，并转义内部的 ' 与 \，避免值被静默切分或错位
func escapeDSNValue(value string) string {
	if !strings.ContainsAny(value, " \t\n\v\f\r") {
		return value
	}
	var builder strings.Builder
	builder.Grow(len(value) + 2)
	builder.WriteByte('\'')
	for _, r := range value {
		switch r {
		case '\'':
			builder.WriteString("\\'")
		case '\\':
			builder.WriteString("\\\\")
		default:
			builder.WriteRune(r)
		}
	}
	builder.WriteByte('\'')
	return builder.String()
}

// withMySQLDefaults 为 MySQL 参数补齐 parseTime=true 与 loc=Local（未显式配置时），
// 避免 orm.BaseModel 的 time.Time 列因缺少 parseTime 读取失败或按 UTC 解析导致时区错位
func withMySQLDefaults(params string) string {
	params = strings.TrimLeft(params, "?&")

	parts := make([]string, 0, 4)
	hasParseTime, hasLoc := false, false
	for _, kv := range strings.Split(params, "&") {
		if kv == "" {
			continue
		}
		parts = append(parts, kv)
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "parseTime":
			hasParseTime = true
		case "loc":
			hasLoc = true
		}
	}
	if !hasParseTime {
		parts = append(parts, "parseTime=true")
	}
	if !hasLoc {
		parts = append(parts, "loc=Local")
	}
	return strings.Join(parts, "&")
}
