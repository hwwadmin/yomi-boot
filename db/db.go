// Package db 提供数据库连接初始化能力
//
// @author zwei
package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/shared/enums"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open 根据配置初始化 DB 连接；调用方应确保 cfg 已通过 Validate。
// 当 min_idle_conns>0 时会预热并启动保活，返回的 stop 须在关闭 DB 前调用（无保活时为 nil）。
func Open(cfg Config) (*gorm.DB, func(), error) {
	return open(cfg)
}

// open 假定 cfg 已通过语义校验，仅处理连接、连接池、预热与保活
func open(cfg Config) (*gorm.DB, func(), error) {
	dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "生成DSN失败")
	}

	var dialector gorm.Dialector
	switch cfg.Type {
	case enums.MySQL:
		dialector = mysql.Open(dsn)
	case enums.Postgres:
		dialector = postgres.Open(dsn)
	default:
		return nil, nil, errors.Newf("不支持的数据库类型: %s", cfg.Type)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: newGormLogger(cfg),
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "初始化数据库连接失败")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, errors.Wrap(err, "获取底层 sql.DB 失败")
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMinutes) * time.Minute)
	idleTimeout := time.Duration(cfg.ConnMaxIdleTimeMinutes) * time.Minute
	sqlDB.SetConnMaxIdleTime(idleTimeout)

	var stop func()
	if cfg.MinIdleConns > 0 {
		warmCtx, cancel := context.WithTimeout(context.Background(), idleWarmTimeout)
		err = warmIdle(warmCtx, sqlDB, cfg.MinIdleConns)
		cancel()
		if err != nil {
			_ = sqlDB.Close()
			return nil, nil, err
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

	return db, stop, nil
}

// buildDSN 根据不同数据库类型返回对应格式的连接串
func buildDSN(cfg Config) (string, error) {
	switch cfg.Type {
	case enums.MySQL:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, withMySQLDefaults(cfg.Params)), nil
	case enums.Postgres:
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s %s", cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.Params), nil
	default:
		return "", errors.Newf("不支持的数据库类型: %s", cfg.Type)
	}
}

// withMySQLDefaults 为 MySQL 参数补齐 parseTime=true 与 loc=Local（未显式配置时），
// 避免 BaseModel 的 time.Time 列因缺少 parseTime 读取失败或按 UTC 解析导致时区错位
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
