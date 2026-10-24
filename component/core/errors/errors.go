package errors

import (
	stderrors "errors"
	"fmt"
	"strings"

	pkgerrors "github.com/pkg/errors"
)

// New 创建全新错误并拍栈（一次性、非共享身份的错误）
func New(text string) error {
	return pkgerrors.New(text)
}

// Newf 格式化创建错误并拍栈
func Newf(format string, args ...any) error {
	return pkgerrors.New(fmt.Sprintf(format, args...))
}

// NewNoStack 创建无调用栈的错误身份（仅用于包级哨兵 var）。
// 返回时须配合 Sentinel，才能得到真实调用点栈。
func NewNoStack(text string) error {
	return stderrors.New(text)
}

// Sentinel 在调用点为预定义哨兵拍栈；Error()/Is 仍指向原哨兵。
// 仅用于「首次返回包级哨兵」；已 Wrap/Sentinel 过的 err 禁止再包。
func Sentinel(err error) error {
	if err == nil {
		return nil
	}
	return pkgerrors.WithStack(err)
}

// Wrap 包装已有错误并拍栈（第三方库错误须使用此方法）
func Wrap(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return pkgerrors.Wrapf(err, format, args...)
}

// Is 判断错误链是否匹配 target
func Is(err, target error) bool {
	return stderrors.Is(err, target)
}

// As 在错误链中查找并赋值 target
func As(err error, target any) bool {
	return stderrors.As(err, target)
}

// AsType 在错误链中查找类型 T 的错误并返回
func AsType[T error](err error) (T, bool) {
	return stderrors.AsType[T](err)
}

// Join 合并多个错误
func Join(errs ...error) error {
	return stderrors.Join(errs...)
}

// Stack 返回错误链；New / Newf / Wrap / Sentinel 创建的节点含 file:line（用于日志）。
// Join 产生的多错误会逐个展开，避免 joinError 的 %+v 只输出消息、丢失各节点的调用栈。
func Stack(err error) string {
	if err == nil {
		return ""
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		parts := multi.Unwrap()
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if s := Stack(part); s != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, "\n")
	}
	return fmt.Sprintf("%+v", err)
}
