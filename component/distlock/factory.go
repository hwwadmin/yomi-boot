// Package distlock DistLockFactory 接口与 NewFactory 构造函数。
//
// @author zwei
package distlock

import redisv9 "github.com/redis/go-redis/v9"

// DistLockFactory 按 Key 创建 Lock 实例
type DistLockFactory interface {
	// GetLock 返回绑定 key 的 Lock，同一 key 可多次创建且彼此独立
	GetLock(key string, opts ...LockOption) Lock
}

// NewFactory 基于 Redis 客户端创建分布式锁工厂
func NewFactory(client *redisv9.Client, opts ...ServiceOption) (DistLockFactory, error) {
	return newRedsyncFactory(client, opts...)
}
