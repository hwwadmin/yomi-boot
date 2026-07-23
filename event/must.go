// Package event Fail Fast 初始化
//
// @author zwei
package event

import (
	"gitee.com/hq_project/yomi-boot/logger"
)

// MustNewAndStart 创建并启动事件组件，失败时 log + panic
func MustNewAndStart(deps Deps, eventCfg Config) *Components {
	if err := eventCfg.Validate(); err != nil {
		logger.ErrorWithoutCtx("event 配置校验失败", "error", err)
		panic(err)
	}
	components, err := NewAndStart(deps, eventCfg)
	if err != nil {
		logger.ErrorWithoutCtx("event 初始化失败", "error", err)
		panic(err)
	}
	return components
}
