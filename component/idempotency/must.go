// Package idempotency Fail Fast 初始化
//
// @author zwei
package idempotency

import (
	"context"

	"gitee.com/hq_project/yomi-boot/core/errors"
	"gitee.com/hq_project/yomi-boot/core/logger"
)

// MustStartStaleCleanerFromConfig 从应用配置创建并启动 Processing 超时清理
// 未启用时返回 nil；已启用但依赖缺失、配置无效或启动失败时 log + panic
func MustStartStaleCleanerFromConfig(parentCtx context.Context, deps StaleCleanerDeps) *StaleCleaner {
	runtimeCfg := StaleCleanerConfigFrom(deps.Config, deps.AppName)
	if !runtimeCfg.Enabled {
		logger.InfoWithoutCtx("[幂等]Processing 超时清理未启用")
		return nil
	}
	if deps.Repo == nil {
		logger.ErrorWithoutCtx("[幂等]Processing 超时清理启动失败", "error", errors.New("数据库未初始化"))
		panic(errors.New("idempotency: 需要 DB"))
	}
	if deps.DistLockFactory == nil {
		logger.ErrorWithoutCtx("[幂等]Processing 超时清理启动失败", "error", errors.New("分布式锁工厂未初始化"))
		panic(errors.New("idempotency: 需要 DistLockFactory"))
	}
	if err := deps.Config.Validate(); err != nil {
		logger.ErrorWithoutCtx("[幂等]Processing 超时清理配置校验失败", "error", err)
		panic(err)
	}
	cleaner := NewStaleCleaner(deps.Repo, runtimeCfg)
	if err := cleaner.Start(parentCtx, deps.DistLockFactory); err != nil {
		panic(err)
	}
	return cleaner
}
