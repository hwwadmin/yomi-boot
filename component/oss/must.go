// Package oss Fail Fast 初始化
//
// @author zwei
package oss

import "gitee.com/hq_project/yomi-boot/core/logger"

// MustNew 创建 Storage，失败时 log + panic
func MustNew(cfg Config) Storage {
	store, err := New(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("oss 初始化失败", "error", err)
		panic(err)
	}
	return store
}
