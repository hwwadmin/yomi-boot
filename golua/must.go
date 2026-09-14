// Package golua Must 构造入口
//
// @author zwei
package golua

import (
	"context"

	"gitee.com/hq_project/yomi-boot/logger"
)

// MustNew 装配组件；失败时记录日志并 panic
func MustNew(ctx context.Context, cfg Config, adapter StoreLoadAdapter) *Components {
	comp, err := New(ctx, cfg, adapter)
	if err != nil {
		logger.ErrorWithoutCtx("golua 初始化失败", "error", err)
		panic(err)
	}
	return comp
}
