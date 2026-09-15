// Package aiclient Fail Fast 构造。
//
// @author zwei
package aiclient

import "gitee.com/hq_project/yomi-boot/logger"

// MustNew 创建 AIClient，失败时记录错误日志并立即崩溃；日志不得包含完整密钥
func MustNew(cfg Config) AIClient {
	cli, err := New(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("aiclient 初始化失败", "error", err, "protocol", string(cfg.Protocol))
		panic(err)
	}
	return cli
}
