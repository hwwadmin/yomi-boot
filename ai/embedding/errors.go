// 定义向量化客户端哨兵错误。
//
// @author zwei

package embedding

import "gitee.com/hq_project/yomi-boot/errors"

var (
	// ErrInvalidArgument 配置或请求未通过本包校验；不含上游 HTTP 400
	ErrInvalidArgument = errors.NewNoStack("AI_EMBEDDING_INVALID_ARGUMENT")
	// ErrUnauthorized 鉴权失败（HTTP 401/403）
	ErrUnauthorized = errors.NewNoStack("AI_EMBEDDING_UNAUTHORIZED")
	// ErrRateLimited 上游限流（HTTP 429）
	ErrRateLimited = errors.NewNoStack("AI_EMBEDDING_RATE_LIMITED")
	// ErrBadResponse 协议或响应体异常（含上游 HTTP 400、无状态码读失败）
	ErrBadResponse = errors.NewNoStack("AI_EMBEDDING_BAD_RESPONSE")
)
