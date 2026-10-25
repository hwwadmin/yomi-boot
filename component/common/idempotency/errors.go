// Package idempotency 提供业务幂等性守卫，确保同一业务请求仅被处理一次
//
// @author zwei
package idempotency

import "gitee.com/hq_project/yomi-boot/component/core/errors"

var (
	// ErrKeyInvalid 幂等键不合法
	ErrKeyInvalid = errors.NewNoStack("sysbiz: invalid key")
	// ErrProcessing 同键请求仍在处理中
	ErrProcessing = errors.NewNoStack("sysbiz: request processing")
	// ErrGuardNotReady 守卫未就绪
	ErrGuardNotReady = errors.NewNoStack("sysbiz: guard not ready")
	// ErrDuplicateWithoutRecord 唯一键冲突但查不到记录
	ErrDuplicateWithoutRecord = errors.NewNoStack("sysbiz: duplicate key without record")
	// ErrNestedTx 调用方 ctx 已绑定同源事务；Execute 须为写路径最外层事务边界，嵌套场景改用 ExecuteInTx
	ErrNestedTx = errors.NewNoStack("sysbiz: nested transaction")
	// ErrNotInTx 调用方 ctx 未绑定同源事务；ExecuteInTx 须在事务内调用，无事务场景改用 Execute
	ErrNotInTx = errors.NewNoStack("sysbiz: not in transaction")
)
