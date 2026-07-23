// Package consul Fail Fast 初始化
//
// @author zwei
package consul

import (
	"yomi-boot/logger"
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

// MustRegisterService 注册服务到 Consul，失败时 log + panic
func MustRegisterService(client *Client, cfg Config, appName string) string {
	serviceID, err := client.RegisterService(cfg, appName)
	if err != nil {
		logger.ErrorWithoutCtx("consul 服务注册失败", "error", err)
		panic(err)
	}
	return serviceID
}
