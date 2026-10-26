// 契约的可执行形式：入参与输出校验，供 Generator 实现与调用方复用。
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

// ValidateResponse 校验拼接输出契约：resp 为 nil 或 Prompt trim 后为空返回 ErrBadResponse；
// 供调用方（如 RAG 编排）核对 Generator 实现输出，不修改 resp
func ValidateResponse(resp *GenerateResponse) error {
	if resp == nil || strings.TrimSpace(resp.Prompt) == "" {
		return errors.Sentinel(ErrBadResponse)
	}
	return nil
}
