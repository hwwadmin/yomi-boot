// Package wsapp 定义门面层哨兵错误
//
// @author zwei
package wsapp

import "gitee.com/hq_project/yomi-boot/errors"

var (
	// ErrInvalidArgument 参数无效
	ErrInvalidArgument = errors.NewNoStack("wsapp: 参数无效")

	// ErrNotConnected 客户端尚未连接或连接已关闭
	ErrNotConnected = errors.NewNoStack("wsapp: 未连接")

	// ErrUnknownPeer 服务端找不到指定 ConnID
	ErrUnknownPeer = errors.NewNoStack("wsapp: 未知会话")
)
