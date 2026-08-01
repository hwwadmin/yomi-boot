// Package redisstream Fail Fast 初始化
//
// @author zwei
package redisstream

import (
	"gitee.com/hq_project/yomi-boot/logger"

	redisv9 "github.com/redis/go-redis/v9"
)

// MustNewComponents 校验配置并创建组件，失败时 log + panic
func MustNewComponents(cfg Config, client *redisv9.Client) *Components {
	components, err := NewComponents(cfg, client)
	if err != nil {
		logger.ErrorWithoutCtx("redisstream 初始化失败", "error", err)
		panic(err)
	}
	return components
}
