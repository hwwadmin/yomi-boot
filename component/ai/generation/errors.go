// 定义提示词拼接门面哨兵错误。
//
// @author zwei

package generation

import "gitee.com/hq_project/yomi-boot/component/core/errors"

var (
	// ErrInvalidArgument 请求未通过本包契约校验
	ErrInvalidArgument = errors.NewNoStack("AI_GENERATION_INVALID_ARGUMENT")
	// ErrBadResponse 拼接实现返回的数据不符合本包契约
	ErrBadResponse = errors.NewNoStack("AI_GENERATION_BAD_RESPONSE")
)
