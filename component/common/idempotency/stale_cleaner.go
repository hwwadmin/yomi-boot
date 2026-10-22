// Package idempotency 提供 Processing 超时清理能力
//
// @author zwei
package idempotency

import (
	"context"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/roundrobin"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const staleCleanerRoundRobinPrefix = "idempotencyStaleProcessing"

// StaleCleanerConfig Processing 超时清理运行时配置
type StaleCleanerConfig struct {
	ScanInterval time.Duration
	StaleTimeout time.Duration
	BatchSize    int
	ServiceName  string
}

// StaleCleanerDeps 一键启动 Processing 超时清理所需依赖
type StaleCleanerDeps struct {
	Repo            *Repository
	DistLockFactory distlock.DistLockFactory
	AppName         string
	Config          Config
}

// StaleCleaner Processing 超时清理器
type StaleCleaner struct {
	repo    *Repository
	cfg     StaleCleanerConfig
	cancel  context.CancelFunc
	started bool
}

// NewStaleCleaner 创建 Processing 超时清理器
func NewStaleCleaner(repo *Repository, cfg StaleCleanerConfig) *StaleCleaner {
	return &StaleCleaner{repo: repo, cfg: cfg}
}

// StaleCleanerConfigFrom 从运行时配置解析 Processing 超时清理配置
func StaleCleanerConfigFrom(cfg Config, appName string) StaleCleanerConfig {
	return StaleCleanerConfig{
		ScanInterval: cfg.ScanIntervalOrDefault(),
		StaleTimeout: cfg.StaleTimeoutOrDefault(),
		BatchSize:    cfg.BatchSizeOrDefault(),
		ServiceName:  strings.TrimSpace(appName),
	}
}

// StartStaleCleanerFromConfig 从运行时配置创建并启动 Processing 超时清理；依赖缺失或启动失败时返回 nil
func StartStaleCleanerFromConfig(parentCtx context.Context, deps StaleCleanerDeps) *StaleCleaner {
	if deps.Repo == nil {
		return nil
	}
	runtimeCfg := StaleCleanerConfigFrom(deps.Config, deps.AppName)
	if deps.DistLockFactory == nil {
		logger.ErrorWithoutCtx("[幂等]Processing 超时清理启动失败", "error", errors.New("分布式锁工厂未初始化"))
		return nil
	}
	cleaner := NewStaleCleaner(deps.Repo, runtimeCfg)
	if err := cleaner.Start(parentCtx, deps.DistLockFactory); err != nil {
		return nil
	}
	return cleaner
}

// Start 启动 Processing 超时清理轮询；启动失败时返回错误
func (c *StaleCleaner) Start(parentCtx context.Context, factory distlock.DistLockFactory) error {
	if c == nil || c.repo == nil {
		return nil
	}
	if c.started {
		return nil
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	runCtx, cancel := context.WithCancel(parentCtx)
	c.cancel = cancel

	name := staleCleanerRoundRobinName(c.cfg.ServiceName)
	if err := roundrobin.Start(runCtx, roundrobin.Simple(name, c.cfg.ScanInterval), c.doTask, factory); err != nil {
		cancel()
		c.cancel = nil
		logger.ErrorWithoutCtx("[幂等]Processing 超时清理启动失败", "name", name, "error", err)
		return err
	}
	c.started = true
	logger.InfoWithoutCtx("[幂等]Processing 超时清理已启动",
		"name", name,
		"scanInterval", c.cfg.ScanInterval.String(),
		"staleTimeout", c.cfg.StaleTimeout.String(),
		"batchSize", c.cfg.BatchSize,
	)
	return nil
}

// Close 停止 Processing 超时清理
func (c *StaleCleaner) Close() error {
	if c == nil || c.cancel == nil {
		return nil
	}
	c.cancel()
	c.cancel = nil
	c.started = false
	return nil
}

// doTask 执行一次超时 Processing 记录清理任务
func (c *StaleCleaner) doTask(ctx context.Context) error {
	before := time.Now().Add(-c.cfg.StaleTimeout)
	count, err := c.repo.DeleteStaleProcessing(ctx, before, c.cfg.BatchSize)
	if err != nil {
		return err
	}
	if count > 0 {
		logger.Warn(ctx, "[幂等]清理超时 Processing 记录",
			"count", count,
			"serviceName", c.cfg.ServiceName,
			"staleTimeout", c.cfg.StaleTimeout.String(),
		)
	}
	return nil
}

// staleCleanerRoundRobinName 构建轮询名称，用于分布式锁 key 隔离
func staleCleanerRoundRobinName(serviceName string) string {
	name := strings.TrimSpace(serviceName)
	if name == "" {
		name = "unknown"
	}
	return staleCleanerRoundRobinPrefix + "_" + name
}
