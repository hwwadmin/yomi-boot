// Package db 提供数据库连接初始化能力
//
// @author zwei
package db

import (
	"fmt"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/shared/enums"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open 根据配置初始化 DB 连接；调用方应确保 cfg 已通过 Validate
func Open(cfg Config) (*gorm.DB, error) {
	return open(cfg)
}

// open 假定 cfg 已通过语义校验，仅处理连接与连接池配置
func open(cfg Config) (*gorm.DB, error) {
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

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, errors.Wrap(err, "初始化数据库连接失败")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, errors.Wrap(err, "获取底层 sql.DB 失败")
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMinutes) * time.Minute)
	sqlDB.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTimeMinutes) * time.Minute)

	logger.InfoWithoutCtx(
		"初始化数据库链接成功",
		"type", string(cfg.Type),
		"addr", cfg.Host+":"+fmt.Sprint(cfg.Port),
		"db", cfg.Database,
	)

	return db, nil
}

// buildDSN 根据不同数据库类型返回对应格式的连接串
func buildDSN(cfg Config) (string, error) {
	switch cfg.Type {
	case enums.MySQL:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.Params), nil
	case enums.Postgres:
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s %s", cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.Params), nil
	default:
		return "", errors.Newf("不支持的数据库类型: %s", cfg.Type)
	}
}
