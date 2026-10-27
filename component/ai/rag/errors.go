// 定义本包哨兵错误。
//
// @author zwei

package rag

import "gitee.com/hq_project/yomi-boot/component/core/errors"

var (
	// ErrInvalidArgument 配置或请求未通过本包校验
	ErrInvalidArgument = errors.NewNoStack("AI_RAG_INVALID_ARGUMENT")
	// ErrBadResponse 节点实现返回的数据不符合契约
	ErrBadResponse = errors.NewNoStack("AI_RAG_BAD_RESPONSE")
)

// wrapSentinel 将本包哨兵与原因同时放入错误链，便于按错误链判断；原因为空时仅拍哨兵栈。
// 错误链风格与 vectordb/llm/embedding/jev 四包一致（errors.Join），修改时需同步五处
func wrapSentinel(sentinel, cause error) error {
	if cause == nil {
		return errors.Sentinel(sentinel)
	}
	return errors.Join(errors.Sentinel(sentinel), errors.Wrap(cause, "ai/rag"))
}
