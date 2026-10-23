// Fail Fast 构造。
//
// @author zwei

package rag

import "gitee.com/hq_project/yomi-boot/component/core/logger"

// MustNew 创建 Engine，失败时记录错误日志并立即崩溃
func MustNew(cfg Config) *Engine {
	engine, err := New(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("ai/rag 初始化失败", "error", err)
		panic(err)
	}
	return engine
}
