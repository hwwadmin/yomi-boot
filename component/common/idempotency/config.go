// Package idempotency 提供业务幂等配置
//
// @author zwei
package idempotency

import (
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	defaultIdempotencyScanIntervalMinutes = 10
	defaultIdempotencyStaleTimeoutMinutes = 60
	defaultIdempotencyBatchSize           = 100
	minIdempotencyScanIntervalMinutes     = 1
	minIdempotencyStaleTimeoutMinutes     = 5
)

// Config 表示业务幂等配置
type Config struct {
	ScanIntervalMinutes int // ScanIntervalMinutes 是清理任务扫描间隔（分钟）
	StaleTimeoutMinutes int // StaleTimeoutMinutes 是 Processing 超时阈值（分钟）
	BatchSize           int // BatchSize 是单轮最多清理条数
}

// ScanIntervalOrDefault 返回清理任务扫描间隔
func (c Config) ScanIntervalOrDefault() time.Duration {
	if c.ScanIntervalMinutes <= 0 {
		return time.Duration(defaultIdempotencyScanIntervalMinutes) * time.Minute
	}
	return time.Duration(c.ScanIntervalMinutes) * time.Minute
}

// StaleTimeoutOrDefault 返回 Processing 超时阈值
func (c Config) StaleTimeoutOrDefault() time.Duration {
	if c.StaleTimeoutMinutes <= 0 {
		return time.Duration(defaultIdempotencyStaleTimeoutMinutes) * time.Minute
	}
	return time.Duration(c.StaleTimeoutMinutes) * time.Minute
}

// BatchSizeOrDefault 返回单轮清理批量大小
func (c Config) BatchSizeOrDefault() int {
	if c.BatchSize <= 0 {
		return defaultIdempotencyBatchSize
	}
	return c.BatchSize
}

// Validate 校验业务幂等配置
func (c Config) Validate() error {
	if c.ScanIntervalMinutes < 0 {
		return errors.New("scan_interval_minutes 不能小于 0")
	}
	if c.StaleTimeoutMinutes < 0 {
		return errors.New("stale_timeout_minutes 不能小于 0")
	}
	if c.BatchSize < 0 {
		return errors.New("batch_size 不能小于 0")
	}
	scan := c.ScanIntervalMinutes
	if scan <= 0 {
		scan = defaultIdempotencyScanIntervalMinutes
	}
	if scan < minIdempotencyScanIntervalMinutes {
		return errors.New("scan_interval_minutes 不能小于 1")
	}
	stale := c.StaleTimeoutMinutes
	if stale <= 0 {
		stale = defaultIdempotencyStaleTimeoutMinutes
	}
	if stale < minIdempotencyStaleTimeoutMinutes {
		return errors.New("stale_timeout_minutes 不能小于 5")
	}
	batch := c.BatchSizeOrDefault()
	if batch <= 0 {
		return errors.New("batch_size 必须大于 0")
	}
	return nil
}
