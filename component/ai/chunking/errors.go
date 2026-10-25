// 定义文本切块策略哨兵错误。
//
// @author zwei

package chunking

import "gitee.com/hq_project/yomi-boot/component/core/errors"

// ErrInvalidArgument 配置、文本或切块输出未通过本包校验
var ErrInvalidArgument = errors.NewNoStack("AI_CHUNKING_INVALID_ARGUMENT")
