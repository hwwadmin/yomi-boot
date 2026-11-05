package errors

// codeWithCause 对外 Error() 为框架哨兵，错误链同时保留哨兵身份与原始原因。
// Sentinel 只拍栈、不保留原因；Wrap 会把原因写进 Error()，从而泄漏到对外 code。
type codeWithCause struct {
	sentinel error
	cause    error
}

// Error 返回哨兵原文，供对外 code 使用。
func (e *codeWithCause) Error() string {
	if e == nil || e.sentinel == nil {
		return ""
	}
	return e.sentinel.Error()
}

// Unwrap 同时暴露哨兵与原始原因，供 errors.Is / errors.As 穿透。
func (e *codeWithCause) Unwrap() []error {
	if e.cause == nil {
		return []error{e.sentinel}
	}
	return []error{e.sentinel, e.cause}
}

// WithCode 对外 Error() 保持哨兵原文，错误链同时保留哨兵与 cause。
// cause 为 nil 时等价于 Sentinel(sentinel)。cause 已经是该哨兵时原样返回，避免再包一层把原因丢掉。
func WithCode(sentinel, cause error) error {
	if sentinel == nil {
		return cause
	}
	if cause == nil {
		return Sentinel(sentinel)
	}
	if Is(cause, sentinel) && cause.Error() == sentinel.Error() {
		return cause
	}
	return &codeWithCause{sentinel: Sentinel(sentinel), cause: cause}
}

// PublicCode 若 err 由 WithCode 收口，返回对外哨兵与原始原因。
func PublicCode(err error) (sentinel, cause error, ok bool) {
	var collapsed *codeWithCause
	if !As(err, &collapsed) || collapsed == nil {
		return nil, nil, false
	}
	return collapsed.sentinel, collapsed.cause, true
}
