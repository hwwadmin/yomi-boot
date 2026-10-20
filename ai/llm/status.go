// Package llm HTTP 状态到本包哨兵的最低映射。
//
// @author zwei
package llm

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"

	"gitee.com/hq_project/yomi-boot/errors"
)

// wrapSentinel 将本包哨兵与上游原因同时放入错误链，便于按错误链判断；原因为空时仅拍哨兵栈
func wrapSentinel(sentinel, cause error) error {
	if cause == nil {
		return errors.Sentinel(sentinel)
	}
	return errors.Join(errors.Sentinel(sentinel), errors.Wrap(cause, "ai/llm"))
}

// mapStatusError 按能取出的 HTTP 状态码映射哨兵并保留原因
//
// 参数：
//   - status: HTTP 状态码；0 表示无法取出
//   - cause: 上游错误
func mapStatusError(status int, cause error) error {
	if cause == nil {
		return nil
	}
	if stderrors.Is(cause, context.Canceled) || stderrors.Is(cause, context.DeadlineExceeded) {
		return wrapCtxErr(cause)
	}
	// 能取出状态码时按最低集合映射哨兵；HTTP 400 归协议异常，与本包入参校验分开
	if status != 0 {
		switch status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return wrapSentinel(ErrUnauthorized, cause)
		case http.StatusTooManyRequests:
			return wrapSentinel(ErrRateLimited, cause)
		}
	}
	// 其它 HTTP 状态、无状态码读失败（含 SSE error）均为协议异常
	return wrapSentinel(ErrBadResponse, cause)
}

// wrapCtxErr 包装取消或超时，保持可按错误链识别标准库错误；已包装则透传
func wrapCtxErr(err error) error {
	if err == nil {
		return nil
	}
	if !stderrors.Is(err, context.Canceled) && !stderrors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// 已包装则透传，避免前缀叠两层
	if stderrors.Unwrap(err) != nil {
		return err
	}
	return errors.Wrap(err, "ai/llm: 请求已取消或超时")
}

// terminateEOF 返回未包装的 io.EOF
func terminateEOF() error {
	return io.EOF
}
