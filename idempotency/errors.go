// Package idempotency 提供业务幂等性守卫，确保同一业务请求仅被处理一次
//
// @author zwei
package idempotency

import "yt-boot/errors"

var (
	// ErrKeyInvalid 幂等键不合法
	ErrKeyInvalid = errors.NewNoStack("sysbiz: invalid key")
	// ErrProcessing 同键请求仍在处理中
	ErrProcessing = errors.NewNoStack("sysbiz: request processing")
	// ErrGuardNotReady 守卫未就绪
	ErrGuardNotReady = errors.NewNoStack("sysbiz: guard not ready")
	// ErrDuplicateWithoutRecord 唯一键冲突但查不到记录
	ErrDuplicateWithoutRecord = errors.NewNoStack("sysbiz: duplicate key without record")
)
