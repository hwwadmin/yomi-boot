// Package consul Fail Fast 初始化
//
// @author zwei
package consul

import (
	"gitee.com/hq_project/yomi-boot/core/logger"
)

// MustNewClient 创建 Consul 客户端，失败时 log + panic
func MustNewClient(cfg Config) *Client {
	client, err := NewClient(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("consul 客户端初始化失败", "error", err)
		panic(err)
	}
	return client
}
