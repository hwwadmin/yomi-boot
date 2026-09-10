// Package wsapp 定义门面配置
//
// @author zwei
package wsapp

import "gitee.com/hq_project/yomi-boot/ws"

const sessionConnIDKey = "__yomi_wsapp_conn_id__"

// ServerConfig 高级入站服务配置。
//
// Handler / Processor 接线规则：
//   - Handler == nil：使用 DefaultServerHandler，其 OnMessage 消费 Processor
//   - Handler != nil：使用自定义 Handler；Config.Processor 不自动接入
//     （自定义 Handler 若仍需 Processor，可嵌入 DefaultServerHandler 自行持有）
type ServerConfig struct {
	ws.ServerConfig // ServerConfig 复用传输层构造参数（业务鉴权勿放在此处，应走连通后报文）

	Handler   ServerHandler          // Handler 可选；nil 则用默认实现
	Processor ServerMessageProcessor // Processor 可选；仅 Handler 为 nil 时由默认实现消费
}

// Validate 校验门面服务配置
func (c ServerConfig) Validate() error {
	return c.ServerConfig.Validate()
}

// resolveHandler 解析实际使用的 Handler
func (c ServerConfig) resolveHandler() ServerHandler {
	if c.Handler != nil {
		return c.Handler
	}
	return DefaultServerHandler{Processor: c.Processor}
}

// ClientConfig 高级出站客户端配置（Handler / Processor 规则同 ServerConfig）
type ClientConfig struct {
	ws.ClientConfig // ClientConfig 复用传输层构造参数

	Handler   ClientHandler          // Handler 可选；nil 则用默认实现
	Processor ClientMessageProcessor // Processor 可选；仅 Handler 为 nil 时由默认实现消费
}

// Validate 校验门面客户端配置
func (c ClientConfig) Validate() error {
	return c.ClientConfig.Validate()
}

// resolveHandler 解析实际使用的 Handler
func (c ClientConfig) resolveHandler() ClientHandler {
	if c.Handler != nil {
		return c.Handler
	}
	return DefaultClientHandler{Processor: c.Processor}
}
