// Package db Fail Fast 初始化
//
// @author zwei
package db

import (
	"yt-boot/logger"

	"gorm.io/gorm"
)

// MustOpen 校验配置并初始化 DB 连接，失败时 log + panic
func MustOpen(cfg Config) *gorm.DB {
	if err := cfg.Validate(); err != nil {
		logger.ErrorWithoutCtx("db 配置校验失败", "error", err)
		panic(err)
	}
	conn, err := open(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("db 初始化失败", "error", err)
		panic(err)
	}
	return conn
}
