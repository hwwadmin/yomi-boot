// Package golua 哨兵错误
//
// @author zwei
package golua

import "gitee.com/hq_project/yomi-boot/errors"

var (
	// ErrDisabled 表示 golua 未启用
	ErrDisabled = errors.NewNoStack("golua: 未启用")
	// ErrNotFound 表示脚本不存在（缓存或数据源）
	ErrNotFound = errors.NewNoStack("golua: 脚本不存在")
	// ErrTimeout 表示执行墙钟超时或父 ctx 取消导致的截止
	ErrTimeout = errors.NewNoStack("golua: 执行超时")
	// ErrCompile 表示脚本编译失败
	ErrCompile = errors.NewNoStack("golua: 编译失败")
	// ErrRuntime 表示脚本运行期错误
	ErrRuntime = errors.NewNoStack("golua: 运行失败")
	// ErrSandbox 表示触碰沙箱限制
	ErrSandbox = errors.NewNoStack("golua: 沙箱拒绝")
	// ErrPoolClosed 表示引擎/池已关闭
	ErrPoolClosed = errors.NewNoStack("golua: 已关闭")
	// ErrPoolAcquireTimeout 表示借 LState 超时
	ErrPoolAcquireTimeout = errors.NewNoStack("golua: 获取执行状态超时")
	// ErrInstructionLimit 表示指令预算耗尽（若运行时具备计数能力）
	ErrInstructionLimit = errors.NewNoStack("golua: 指令上限")
)
