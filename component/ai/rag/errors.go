// 定义检索增强数据能力哨兵错误。
//
// @author zwei

package rag

import "gitee.com/hq_project/yomi-boot/component/core/errors"

var (
	// ErrInvalidArgument 配置或请求未通过本包校验
	ErrInvalidArgument = errors.NewNoStack("AI_RAG_INVALID_ARGUMENT")
	// ErrDimensionMismatch 向量化输出维度与本实例锁定的维度不一致
	ErrDimensionMismatch = errors.NewNoStack("AI_RAG_DIMENSION_MISMATCH")
	// ErrBadResponse 向量化实现或上游返回的数据不符合本包契约
	ErrBadResponse = errors.NewNoStack("AI_RAG_BAD_RESPONSE")
)

// sentinelWithCause 以哨兵作为 Error() 身份，同时保留原因链
type sentinelWithCause struct {
	sentinel error // sentinel 是线上 code
	cause    error // cause 是原始原因
}

// Error 返回哨兵码，避免原因文本污染线上 code
func (e *sentinelWithCause) Error() string {
	return e.sentinel.Error()
}

// Unwrap 同时暴露哨兵与原因，支持 errors.Is/As 与 errors.Stack
func (e *sentinelWithCause) Unwrap() []error {
	return []error{e.sentinel, e.cause}
}

// wrapSentinel 返回以哨兵为 Error() 的错误链；cause 为空时仅拍哨兵栈
func wrapSentinel(sentinel, cause error) error {
	if cause == nil {
		return errors.Sentinel(sentinel)
	}
	return &sentinelWithCause{sentinel: errors.Sentinel(sentinel), cause: cause}
}
