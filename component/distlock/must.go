// Package distlock Fail Fast 初始化
//
// @author zwei
package distlock

import (
	"gitee.com/hq_project/yomi-boot/core/logger"

	redisv9 "github.com/redis/go-redis/v9"
)

// MustNewFactory 创建分布式锁工厂，失败时 log + panic
func MustNewFactory(client *redisv9.Client, opts ...ServiceOption) DistLockFactory {
	factory, err := NewFactory(client, opts...)
	if err != nil {
		logger.ErrorWithoutCtx("distlock 初始化失败", "error", err)
		panic(err)
	}
	return factory
}
