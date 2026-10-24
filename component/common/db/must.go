// 数据源 Fail Fast 初始化
//
// @author zwei

package db

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// MustOpen 校验配置并初始化数据源，失败时 log + panic
func MustOpen(ctx context.Context, cfg Config) *Conn {
	if err := cfg.Validate(); err != nil {
		logger.ErrorWithoutCtx("db 配置校验失败", "error", err)
		panic(err)
	}
	conn, err := open(ctx, cfg)
	if err != nil {
		logger.ErrorWithoutCtx("db 初始化失败", "error", err)
		panic(err)
	}
	return conn
}
