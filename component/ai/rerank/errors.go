// 定义重排门面哨兵错误。
//
// @author zwei

package rerank

import "gitee.com/hq_project/yomi-boot/component/core/errors"

var (
	// ErrInvalidArgument 配置或请求未通过本包校验
	ErrInvalidArgument = errors.NewNoStack("AI_RERANK_INVALID_ARGUMENT")
	// ErrBadResponse 重排实现返回的数据不符合本包契约
	ErrBadResponse = errors.NewNoStack("AI_RERANK_BAD_RESPONSE")
)
