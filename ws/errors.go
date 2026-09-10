// Package ws 定义 WebSocket 相关哨兵错误
//
// @author zwei
package ws

import "gitee.com/hq_project/yomi-boot/errors"

var (
	// ErrInvalidArgument 构造参数或选项无效
	ErrInvalidArgument = errors.NewNoStack("ws: 参数无效")

	// ErrClosed 服务或连接已关闭
	ErrClosed = errors.NewNoStack("ws: 已关闭")

	// ErrNotStarted 服务尚未 Start
	ErrNotStarted = errors.NewNoStack("ws: 未启动")
)
