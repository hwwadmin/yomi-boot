// Package roundrobin 提供分布式环境下的定时轮询调度能力
//
// @author zwei
package roundrobin

import (
	"context"
	"time"
	"yomi-boot/errors"
)

const minInterval = 50 * time.Millisecond // minInterval 是轮询相关时间参数的最小合法值

// Config 轮询策略配置
type Config struct {
	Name           string        // Name 轮询任务名，同服务集群各实例共用，用于 distlock Key 后缀以实现集群互斥
	Frequency      time.Duration // Frequency 轮询频率，同时作为频率锁 TTL
	RandomDelayMin time.Duration // RandomDelayMin 成功执行后的随机延迟下限
	RandomDelayMax time.Duration // RandomDelayMax 成功执行后的随机延迟上限，须大于 RandomDelayMin
	WakeUpInterval time.Duration // WakeUpInterval 抢锁失败或短睡间隔
}

// Task 单次轮询任务
type Task func(ctx context.Context) error

// Simple 创建带默认延迟与唤醒间隔的轮询配置
func Simple(name string, frequency time.Duration) Config {
	wakeUp := frequency / 2
	if wakeUp <= 100*time.Millisecond {
		wakeUp = 100 * time.Millisecond
	}
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
	if cfg.Frequency < minInterval {
		return errors.New("roundrobin.frequency 必须大于等于 50ms")
	}
	if cfg.RandomDelayMin < minInterval {
		return errors.New("roundrobin.random_delay_min 必须大于等于 50ms")
	}
	if cfg.RandomDelayMax <= cfg.RandomDelayMin {
		return errors.New("roundrobin.random_delay_max 必须大于 random_delay_min")
	}
	if cfg.WakeUpInterval < minInterval {
		return errors.New("roundrobin.wake_up_interval 必须大于等于 50ms")
	}
	return nil
}
