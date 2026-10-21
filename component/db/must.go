// Package db Fail Fast 初始化
//
// @author zwei
package db

import (
	"gitee.com/hq_project/yomi-boot/boot/logger"

	"gorm.io/gorm"
)

// MustOpen 校验配置并初始化 DB 连接，失败时 log + panic。
// 未启用（enabled=false，默认）时返回 (nil, nil)，调用方须先判断 nil 再使用连接。
// 返回的 stop 用于停止最小空闲保活，关闭 DB 前调用；无保活时为 nil。
func MustOpen(cfg Config) (*gorm.DB, func()) {
	if !cfg.EnabledOrDefault() {
		logger.InfoWithoutCtx("db 未启用，跳过初始化")
		return nil, nil
	}
	if err := cfg.Validate(); err != nil {
		logger.ErrorWithoutCtx("db 配置校验失败", "error", err)
		panic(err)
	}
	conn, stop, err := open(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("db 初始化失败", "error", err)
		panic(err)
	}
	return conn, stop
}
