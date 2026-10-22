// Package errors 提供业务异常类型，用于业务域携带可选 code
//
// 框架库请使用各包自行维护的简单哨兵 error，不要使用 BizError 定义框架错误。
//
// @author zwei
package errors

import "fmt"

// BizError 业务域异常，可携带对外 code；实现 error，支持 errors.Is/As
type BizError struct {
	Code    string // Code 是对外业务错误码（由业务与 Encoder 约定）
	Message string // Message 是开发调试消息
	Cause   error  // Cause 是原始错误
}

// Error 返回格式化错误消息
func (e *BizError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return "[" + e.Code + "] " + e.Message
	}
	return "[" + e.Code + "]"
}

// Unwrap 支持 errors.Is / errors.As 穿透
func (e *BizError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is 按 Code 匹配
func (e *BizError) Is(target error) bool {
	if e == nil {
		return false
	}
	other, ok := target.(*BizError)
	if !ok || other == nil {
		return false
	}
	return e.Code == other.Code
}

// NewBiz 创建业务异常哨兵
func NewBiz(code, msg string) *BizError {
	return &BizError{Code: code, Message: msg}
}

// NewBizf 创建带格式化消息的业务异常
func NewBizf(code, format string, args ...any) *BizError {
	return &BizError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WrapBiz 包装原始错误为业务异常
func WrapBiz(cause error, code, msg string) *BizError {
	return &BizError{Code: code, Message: msg, Cause: cause}
}
