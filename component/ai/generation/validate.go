// 契约的可执行形式：入参校验，供 Generator 实现复用。
//
// @author zwei

package generation

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// ValidateRequest 校验拼接请求：Query 与每条 Context.Content trim 后非空；
// 供 Generator 实现自查入参契约，非法返回 ErrInvalidArgument
func ValidateRequest(req GenerateRequest) error {
	if strings.TrimSpace(req.Query) == "" {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for _, c := range req.Contexts {
		if strings.TrimSpace(c.Content) == "" {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}
