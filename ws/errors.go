// Package ws 定义哨兵错误
//
// @author zwei
package ws

import "gitee.com/hq_project/yomi-boot/errors"

var (
	// ErrInvalidArgument 参数无效
	ErrInvalidArgument = errors.NewNoStack("ws: 参数无效")

	// ErrNotConnected 客户端尚未连接或连接已关闭
	ErrNotConnected = errors.NewNoStack("ws: 未连接")

	// ErrUnknownPeer 服务端找不到指定 ConnID
	ErrUnknownPeer = errors.NewNoStack("ws: 未知会话")

	// ErrClosed 服务已关闭（Server 拒绝新入队）
	ErrClosed = errors.NewNoStack("ws: 已关闭")

	// ErrNotStarted 服务尚未 Start
	ErrNotStarted = errors.NewNoStack("ws: 未启动")
)
