// Package filestorage Fail Fast 初始化
//
// @author zwei
package filestorage

import "gitee.com/hq_project/yomi-boot/logger"

// MustNew 创建 FileStorage，失败时 log + panic
func MustNew(cfg Config) FileStorage {
	store, err := New(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("filestorage 初始化失败", "error", err)
		panic(err)
	}
	return store
}
