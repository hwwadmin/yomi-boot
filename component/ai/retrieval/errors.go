// 定义检索门面哨兵错误。
//
// @author zwei

package retrieval

import "gitee.com/hq_project/yomi-boot/component/core/errors"

var (
	// ErrInvalidArgument 配置或请求未通过本包校验
	ErrInvalidArgument = errors.NewNoStack("AI_RETRIEVAL_INVALID_ARGUMENT")
	// ErrBadResponse 向量化实现或上游返回的数据不符合本包契约
	ErrBadResponse = errors.NewNoStack("AI_RETRIEVAL_BAD_RESPONSE")
)
