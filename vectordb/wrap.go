// Package vectordb 错误链辅助。
//
// @author zwei
package vectordb

import (
	"context"
	stderrors "errors"

	"gitee.com/hq_project/yomi-boot/errors"
)

// wrapSentinel 将本包哨兵与原因同时放入错误链
func wrapSentinel(sentinel, cause error) error {
	if cause == nil {
		return errors.Sentinel(sentinel)
	}
	return errors.Join(errors.Sentinel(sentinel), errors.Wrap(cause, "vectordb"))
}

// wrapCtxErr 包装取消或超时
func wrapCtxErr(err error) error {
	if err == nil {
		return nil
	}
	if !stderrors.Is(err, context.Canceled) && !stderrors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if stderrors.Unwrap(err) != nil {
		return err
	}
	return errors.Wrap(err, "vectordb: 请求已取消或超时")
}

// mapEngineError 将引擎错误映射为本包哨兵；ctx 错误优先
func mapEngineError(err error) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return wrapCtxErr(err)
	}
	if stderrors.Is(err, ErrInvalidArgument) || stderrors.Is(err, ErrAlreadyExists) ||
		stderrors.Is(err, ErrNotFound) || stderrors.Is(err, ErrClosed) || stderrors.Is(err, ErrUnavailable) {
		return err
	}
	return wrapSentinel(ErrUnavailable, err)
}
