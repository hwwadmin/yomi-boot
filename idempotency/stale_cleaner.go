// Package idempotency 提供 Processing 超时清理能力
//
// @author zwei
package idempotency

import (
	"context"
	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/roundrobin"
	"gitee.com/hq_project/yomi-boot/runner"
	"strings"
	"time"
)

const staleCleanerRoundRobinPrefix = "idempotencyStaleProcessing"

// StaleCleanerConfig Processing 超时清理运行时配置
type StaleCleanerConfig struct {
	Enabled      bool
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

// StaleCleanerConfigFrom 从应用配置解析 Processing 超时清理运行时配置
func StaleCleanerConfigFrom(cfg Config, appName string) StaleCleanerConfig {
	return StaleCleanerConfig{
		Enabled:      cfg.StaleCleanerEnabledOrDefault(),
		ScanInterval: cfg.ScanIntervalOrDefault(),
		StaleTimeout: cfg.StaleTimeoutOrDefault(),
		BatchSize:    cfg.BatchSizeOrDefault(),
		ServiceName:  strings.TrimSpace(appName),
	}
}

// StartStaleCleanerFromConfig 从应用配置创建并启动 Processing 超时清理；未启用或依赖缺失时返回 nil
func StartStaleCleanerFromConfig(parentCtx context.Context, deps StaleCleanerDeps) *StaleCleaner {
	if deps.Repo == nil {
		return nil
	}
	runtimeCfg := StaleCleanerConfigFrom(deps.Config, deps.AppName)
	if !runtimeCfg.Enabled {
		logger.InfoWithoutCtx("[幂等]Processing 超时清理未启用")
		return nil
	}
	if deps.DistLockFactory == nil {
		logger.ErrorWithoutCtx("[幂等]Processing 超时清理启动失败", "error", errors.New("分布式锁工厂未初始化"))
		return nil
	}
	cleaner := NewStaleCleaner(deps.Repo, runtimeCfg)
	cleaner.Start(parentCtx, deps.DistLockFactory)
	return cleaner
}

// AttachStaleCleanerCleanup 将 Processing 超时清理器挂入服务 Cleanup 链；通常通过 SetupStaleCleanerOnBundle 调用
func AttachStaleCleanerCleanup(bundle *runner.ServerBundle, cleaner *StaleCleaner) {
	if bundle == nil || cleaner == nil {
		return
	}
	original := bundle.Cleanup
	bundle.Cleanup = func() {
		_ = cleaner.Close()
		if original != nil {
			original()
		}
	}
}

// SetupStaleCleanerOnBundle 在 ServerBundle 就绪后启动 Processing 超时清理，并挂入 Cleanup 链
func SetupStaleCleanerOnBundle(bundle *runner.ServerBundle, parentCtx context.Context, deps StaleCleanerDeps) {
	cleaner := StartStaleCleanerFromConfig(parentCtx, deps)
	AttachStaleCleanerCleanup(bundle, cleaner)
}

// Start 启动 Processing 超时清理轮询
func (c *StaleCleaner) Start(parentCtx context.Context, factory distlock.DistLockFactory) {
	if c == nil || c.repo == nil || !c.cfg.Enabled {
		return
	}
	if c.started {
		return
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	runCtx, cancel := context.WithCancel(parentCtx)
	c.cancel = cancel

	name := staleCleanerRoundRobinName(c.cfg.ServiceName)
	roundrobin.Start(runCtx, roundrobin.Simple(name, c.cfg.ScanInterval), c.doTask, factory)
	c.started = true
	logger.InfoWithoutCtx("[幂等]Processing 超时清理已启动",
		"name", name,
		"scanInterval", c.cfg.ScanInterval.String(),
		"staleTimeout", c.cfg.StaleTimeout.String(),
		"batchSize", c.cfg.BatchSize,
	)
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
