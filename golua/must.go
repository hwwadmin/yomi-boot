// Package golua Fail Fast 初始化
//
// @author zwei
package golua

import (
	"context"

	"gitee.com/hq_project/yomi-boot/logger"

	redisv9 "github.com/redis/go-redis/v9"
)

// MustNew 装配 golua 组件，失败时 log + panic
func MustNew(ctx context.Context, cfg Config, adapter StoreLoadAdapter, rdb *redisv9.Client) *Components {
	comp, err := New(ctx, cfg, adapter, rdb)
	if err != nil {
		logger.ErrorWithoutCtx("golua 初始化失败", "error", err)
		panic(err)
	}
	return comp
}
