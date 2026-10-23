// Package logger access 方向常量
//
// @author zwei
package logger

// Access 日志 direction 取值（同一次 HTTP：进入=inbound，离开=outbound）
const (
	// DirectionInbound 请求进入业务 handler 前
	DirectionInbound = "inbound"
	// DirectionOutbound 响应离开业务 handler 后
	DirectionOutbound = "outbound"
)
