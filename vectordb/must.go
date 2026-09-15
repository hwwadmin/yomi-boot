// Package vectordb Fail Fast 构造。
//
// @author zwei
package vectordb

import "gitee.com/hq_project/yomi-boot/logger"

// MustNew 创建 Store，失败时记录错误日志并立即崩溃；日志不得包含完整密钥
func MustNew(cfg Config) Store {
	store, err := New(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("vectordb 初始化失败", "error", err, "provider", string(cfg.Provider))
		panic(err)
	}
	return store
}
