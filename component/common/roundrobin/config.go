// Package roundrobin 提供分布式环境下的定时轮询调度能力
//
// @author zwei
package roundrobin

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const MinInterval = 50 * time.Millisecond // MinInterval 是轮询相关时间参数的最小合法值

// Config 轮询策略配置
type Config struct {
	Name           string        // Name 轮询任务名，同服务集群各实例共用，用于 distlock Key 后缀以实现集群互斥
	Frequency      time.Duration // Frequency 轮询频率，同时作为频率锁 TTL
	RandomDelayMin time.Duration // RandomDelayMin 成功执行后的随机延迟下限
	RandomDelayMax time.Duration // RandomDelayMax 成功执行后的随机延迟上限，须大于 RandomDelayMin
	WakeUpInterval time.Duration // WakeUpInterval 抢锁失败或短睡间隔
	MaxRunDuration time.Duration // MaxRunDuration 单次任务最长执行时间，0 表示不限制；同时作为执行锁 MaxHoldDuration
}

// Task 单次轮询任务
type Task func(ctx context.Context) error

// Simple 创建带默认延迟与唤醒间隔的轮询配置；MaxRunDuration 默认为 0（不限制）
func Simple(name string, frequency time.Duration) Config {
	wakeUp := max(frequency/2, 100*time.Millisecond)
	return Config{
		Name:           name,
		Frequency:      frequency,
		RandomDelayMin: 100 * time.Millisecond,
		RandomDelayMax: 100*time.Millisecond + frequency,
		WakeUpInterval: wakeUp,
	}
}

// validate 校验轮询配置
func (cfg Config) validate() error {
	if cfg.Name == "" {
		return errors.New("roundrobin.name 不能为空")
	}
	if cfg.Frequency < MinInterval {
		return errors.New("roundrobin.frequency 必须大于等于 50ms")
	}
	if cfg.RandomDelayMin < MinInterval {
		return errors.New("roundrobin.random_delay_min 必须大于等于 50ms")
	}
	if cfg.RandomDelayMax <= cfg.RandomDelayMin {
		return errors.New("roundrobin.random_delay_max 必须大于 random_delay_min")
	}
	if cfg.WakeUpInterval < MinInterval {
		return errors.New("roundrobin.wake_up_interval 必须大于等于 50ms")
	}
	if cfg.MaxRunDuration < 0 {
		return errors.New("roundrobin.max_run_duration 不能为负数")
	}
	return nil
}
