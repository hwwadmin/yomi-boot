// Package golua 哨兵错误
//
// @author zwei
package golua

import "gitee.com/hq_project/yomi-boot/errors"

var (
	// ErrNotFound 脚本不存在
	ErrNotFound = errors.NewNoStack("golua: 脚本不存在")
	// ErrTimeout 执行超时
	ErrTimeout = errors.NewNoStack("golua: 执行超时")
	// ErrCompile 编译失败
	ErrCompile = errors.NewNoStack("golua: 编译失败")
	// ErrRuntime 运行期错误
	ErrRuntime = errors.NewNoStack("golua: 运行失败")
	// ErrSandbox 触碰沙箱限制
	ErrSandbox = errors.NewNoStack("golua: 沙箱拒绝")
	// ErrPoolClosed 引擎或池已关闭
	ErrPoolClosed = errors.NewNoStack("golua: 已关闭")
	// ErrPoolAcquireTimeout 借 LState 超时
	ErrPoolAcquireTimeout = errors.NewNoStack("golua: 获取执行状态超时")
	// ErrInstructionLimit 指令预算耗尽
	ErrInstructionLimit = errors.NewNoStack("golua: 指令上限")
)
