// Package db Fail Fast 初始化
//
// @author zwei
package db

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/core/logger"

	"gorm.io/gorm"
)

// MustOpen 校验配置并初始化 DB 连接，失败时 log + panic。
// 返回的 stop 用于停止最小空闲保活，关闭 DB 前调用；无保活时为 nil。
func MustOpen(ctx context.Context, cfg Config) (*gorm.DB, func()) {
	if err := cfg.Validate(); err != nil {
		logger.ErrorWithoutCtx("db 配置校验失败", "error", err)
		panic(err)
	}
	conn, stop, err := open(ctx, cfg)
	if err != nil {
		logger.ErrorWithoutCtx("db 初始化失败", "error", err)
		panic(err)
	}
	return conn, stop
}
