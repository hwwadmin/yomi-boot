// Package wsapp 定义门面配置
//
// @author zwei
package wsapp

import "gitee.com/hq_project/yomi-boot/ws"

const sessionConnIDKey = "__yomi_wsapp_conn_id__"

// ServerConfig 高级入站服务配置
type ServerConfig struct {
	ws.ServerConfig // ServerConfig 复用传输层构造参数（业务鉴权勿放在此处，应走连通后报文）
}

// Validate 校验门面服务配置
func (c ServerConfig) Validate() error {
	return c.ServerConfig.Validate()
}

// ClientConfig 高级出站客户端配置
type ClientConfig struct {
	ws.ClientConfig // ClientConfig 复用传输层构造参数
}

// Validate 校验门面客户端配置
func (c ClientConfig) Validate() error {
	return c.ClientConfig.Validate()
}
