// Package error 定义 yomi-boot 跨库通用哨兵错误（error.Error() 即为线上 code）
//
// 仅放多个库都会用到的通用错误；库私有错误请在各自包内就近定义。
//
// @author zwei
package error

import "yomi-boot/errors"

var (
	// ErrInternal 系统内部错误
	ErrInternal = errors.NewNoStack("INTERNAL_ERROR")
	// ErrInvalidBody 请求体格式不合法
	ErrInvalidBody = errors.NewNoStack("INVALID_BODY")
	// ErrInvalidInput 请求参数不合法
	ErrInvalidInput = errors.NewNoStack("INVALID_INPUT")
	// ErrRequestTimeout 请求处理超时
	ErrRequestTimeout = errors.NewNoStack("REQUEST_TIMEOUT")
	// ErrRequestBodyTooLarge 请求体过大
	ErrRequestBodyTooLarge = errors.NewNoStack("REQUEST_BODY_TOO_LARGE")
)
