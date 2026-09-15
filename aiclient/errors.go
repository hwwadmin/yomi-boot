// Package aiclient 定义大模型客户端哨兵错误。
//
// @author zwei
package aiclient

import "gitee.com/hq_project/yomi-boot/errors"

var (
	// ErrInvalidArgument 配置或请求参数不合法
	ErrInvalidArgument = errors.NewNoStack("AICLIENT_INVALID_ARGUMENT")
	// ErrUnauthorized 鉴权失败（HTTP 401/403）
	ErrUnauthorized = errors.NewNoStack("AICLIENT_UNAUTHORIZED")
	// ErrRateLimited 上游限流（HTTP 429）
	ErrRateLimited = errors.NewNoStack("AICLIENT_RATE_LIMITED")
	// ErrBadResponse 协议或响应体异常
	ErrBadResponse = errors.NewNoStack("AICLIENT_BAD_RESPONSE")
)
