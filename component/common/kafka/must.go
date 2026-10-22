// Package kafka Fail Fast 初始化
//
// @author zwei
package kafka

import (
	"gitee.com/hq_project/yomi-boot/component/common/logger"
)

// MustNewComponents 校验配置并创建 Kafka 组件，失败时 log + panic
func MustNewComponents(cfg Config) *Components {
	if err := cfg.Validate(); err != nil {
		logger.ErrorWithoutCtx("kafka 配置校验失败", "error", err)
		panic(err)
	}
	components, err := NewComponents(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("kafka 初始化失败", "error", err)
		panic(err)
	}
	return components
}
