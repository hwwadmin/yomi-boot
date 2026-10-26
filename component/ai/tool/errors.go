// 定义对外错误、错误码与安全文案。
//
// @author zwei

package tool

import (
	"context"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	// CodeInvalidArgument 工具定义、调用参数或配置未通过校验
	CodeInvalidArgument = "AI_TOOL_INVALID_ARGUMENT"
	// CodeToolNotFound 工具名未注册
	CodeToolNotFound = "AI_TOOL_NOT_FOUND"
	// CodeExecution 工具实现返回错误
	CodeExecution = "AI_TOOL_EXECUTION"
	// CodeTimeout 调用超时
	CodeTimeout = "AI_TOOL_TIMEOUT"
	// CodeCanceled 调用取消
	CodeCanceled = "AI_TOOL_CANCELED"
	// CodePanic 工具实现触发 panic
	CodePanic = "AI_TOOL_PANIC"
	// CodeBadResponse 工具返回内容不符合契约
	CodeBadResponse = "AI_TOOL_BAD_RESPONSE"
	// CodeFillFailed 填槽未配置模型或模型调用失败
	CodeFillFailed = "AI_TOOL_FILL_FAILED"
	// CodeFillInvalid 模型输出无法解析或不符合工具入参契约
	CodeFillInvalid = "AI_TOOL_FILL_INVALID"
	// CodeInputIncomplete 工具入参缺槽
	CodeInputIncomplete = "AI_TOOL_INPUT_INCOMPLETE"
)

const (
	// maxSafeMessageRunes 是对外安全文案的最大字符数，避免错误信息撑爆响应
	maxSafeMessageRunes = 512
	// maxErrorFieldRunes 是对外错误中工具名与调用标识的最大字符数
	maxErrorFieldRunes = 256
	// maxMissingKeys 是对外错误中缺失槽位的最大数量
	maxMissingKeys = 64

	// defaultInvalidDefinitionMessage 工具定义不合法
	defaultInvalidDefinitionMessage = "工具定义不合法"
	// defaultInvalidConfigMessage 工具配置不合法
	defaultInvalidConfigMessage = "工具配置不合法"
	// defaultInvalidArgumentMessage 工具参数不合法
	defaultInvalidArgumentMessage = "工具参数不合法"
	// defaultToolNotFoundMessage 工具未注册
	defaultToolNotFoundMessage = "工具未注册"
	// defaultExecutionMessage 工具执行失败
	defaultExecutionMessage = "工具执行失败"
	// defaultTimeoutMessage 工具执行超时
	defaultTimeoutMessage = "工具执行超时"
	// defaultCanceledMessage 工具执行已取消
	defaultCanceledMessage = "工具执行已取消"
	// defaultPanicMessage 工具执行异常
	defaultPanicMessage = "工具执行异常"
	// defaultBadResponseMessage 工具返回内容不合法
	defaultBadResponseMessage = "工具返回内容不合法"
	// defaultArgumentsTooLargeMessage 工具参数超过大小上限
	defaultArgumentsTooLargeMessage = "工具参数超过大小上限"
	// defaultContentTooLargeMessage 工具返回内容超过大小上限
	defaultContentTooLargeMessage = "工具返回内容超过大小上限"
	// defaultFillFailedMessage 工具入参生成失败
	defaultFillFailedMessage = "工具入参生成失败"
	// defaultFillInvalidMessage 工具入参生成结果不合法
	defaultFillInvalidMessage = "工具入参生成结果不合法"
	// defaultInputIncompleteMessage 工具入参不完整
	defaultInputIncompleteMessage = "工具入参不完整"
)

var (
	// ErrInvalidArgument 工具定义、调用参数或配置未通过校验
	ErrInvalidArgument = errors.NewNoStack(CodeInvalidArgument)
	// ErrToolNotFound 工具名未注册
	ErrToolNotFound = errors.NewNoStack(CodeToolNotFound)
	// ErrExecution 执行类失败总哨兵，覆盖执行错误、超时、取消、panic 与非法输出
	ErrExecution = errors.NewNoStack(CodeExecution)
	// ErrPanic 工具实现触发 panic
	ErrPanic = errors.NewNoStack(CodePanic)
	// ErrBadResponse 工具返回内容不符合契约
	ErrBadResponse = errors.NewNoStack(CodeBadResponse)
	// ErrFillFailed 填槽未配置模型或模型调用失败
	ErrFillFailed = errors.NewNoStack(CodeFillFailed)
	// ErrFillInvalid 模型输出无法解析或不符合工具入参契约
	ErrFillInvalid = errors.NewNoStack(CodeFillInvalid)
	// ErrInputIncomplete 工具入参缺槽
	ErrInputIncomplete = errors.NewNoStack(CodeInputIncomplete)
)

// SafeError 由工具实现的安全错误可选实现；库只把 SafeMessage 暴露给外部，原始错误仍只进 Cause。
// 实现方须保证返回内容不含堆栈、SQL、内部地址等敏感信息。
type SafeError interface {
	// SafeMessage 返回可对外展示的安全文案
	SafeMessage() string
}

// Error 是对外错误；Code、Message、Tool、CallID 可安全用于日志或模型消息。
// 底层原因不进入 Error()、不参与 JSON 序列化，也不实现 Unwrap；仅能经 Cause 显式获取。
type Error struct {
	Code    string   `json:"code"`              // Code 是稳定对外错误码
	Message string   `json:"message"`           // Message 是安全对外文案，不含底层错误文本
	Tool    string   `json:"tool,omitempty"`    // Tool 是相关工具名；构造错误时可能为空
	CallID  string   `json:"call_id,omitempty"` // CallID 是调用标识；构造错误时为空
	Missing []string `json:"missing,omitempty"` // Missing 是缺失槽位键；仅缺槽错误使用

	cause error // cause 是底层原因，仅进程内日志/告警可用
}

var (
	_ error     = (*Error)(nil)
	_ SafeError = (*Error)(nil)
)

// Error 返回稳定的错误码与安全文案；绝不含底层原因
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return "[" + e.Code + "]"
	}
	return "[" + e.Code + "] " + e.Message
}

// Is 按错误码匹配本包哨兵；不穿透底层原因
func (e *Error) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	switch target {
	case ErrInvalidArgument:
		return e.Code == CodeInvalidArgument
	case ErrToolNotFound:
		return e.Code == CodeToolNotFound
	case ErrExecution:
		return e.isExecution()
	case ErrPanic:
		return e.Code == CodePanic
	case ErrBadResponse:
		return e.Code == CodeBadResponse
	case ErrFillFailed:
		return e.Code == CodeFillFailed
	case ErrFillInvalid:
		return e.Code == CodeFillInvalid
	case ErrInputIncomplete:
		return e.Code == CodeInputIncomplete
	case context.Canceled:
		return e.Code == CodeCanceled
	case context.DeadlineExceeded:
		return e.Code == CodeTimeout
	}
	// 同类型错误按 Code 判等，便于调用方构造期望错误做断言
	other, ok := target.(*Error)
	return ok && other != nil && e.Code == other.Code
}

// Cause 返回底层原因，仅供进程内日志/告警使用；
// 禁止写入 API 响应、消息队列、模型消息、前端或任何对外可见的位置。
func (e *Error) Cause() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// SafeMessage 实现 SafeError，返回安全对外文案；嵌套工具错误可据此保留可展示信息
func (e *Error) SafeMessage() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// isExecution 判断错误码是否属于执行类失败
func (e *Error) isExecution() bool {
	switch e.Code {
	case CodeExecution, CodeTimeout, CodeCanceled, CodePanic, CodeBadResponse:
		return true
	default:
		return false
	}
}

// isKnownCode 判断错误码是否为本包公开错误码
func isKnownCode(code string) bool {
	switch code {
	case CodeInvalidArgument, CodeToolNotFound, CodeExecution, CodeTimeout, CodeCanceled,
		CodePanic, CodeBadResponse, CodeFillFailed, CodeFillInvalid, CodeInputIncomplete:
		return true
	default:
		return false
	}
}

// newError 构造对外错误；文案为空时按错误码填默认值，并截断到安全长度
func newError(code, message, toolName, callID string, cause error) *Error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = defaultMessage(code)
	}
	return &Error{
		Code:    code,
		Message: truncateRunes(message, maxSafeMessageRunes),
		Tool:    truncateRunes(strings.TrimSpace(toolName), maxErrorFieldRunes),
		CallID:  truncateRunes(callID, maxErrorFieldRunes),
		cause:   cause,
	}
}

// newIncompleteError 构造缺槽错误；Missing 去重、trim、截断
func newIncompleteError(toolName, callID string, missing []string) *Error {
	err := newError(CodeInputIncomplete, defaultInputIncompleteMessage, toolName, callID, nil)
	err.Missing = normalizeMissing(missing)
	return err
}

// normalizeMissing 规范化缺失槽位键：去空、去重、截断
func normalizeMissing(missing []string) []string {
	if len(missing) == 0 {
		return nil
	}
	out := make([]string, 0, len(missing))
	seen := make(map[string]struct{}, len(missing))
	for _, key := range missing {
		key = truncateRunes(strings.TrimSpace(key), maxErrorFieldRunes)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
		if len(out) == maxMissingKeys {
			break
		}
	}
	return out
}

// withCallContext 用当前工具/调用标识重写本包错误的上下文；非本包错误按执行失败包装
func withCallContext(err error, toolName, callID string) *Error {
	if err == nil {
		return nil
	}
	var existing *Error
	if errors.As(err, &existing) && existing != nil {
		out := newError(existing.Code, existing.SafeMessage(), toolName, callID, existing.Cause())
		out.Missing = normalizeMissing(existing.Missing)
		return out
	}
	return newError(CodeExecution, safeMessage(err, defaultExecutionMessage), toolName, callID, err)
}

// defaultMessage 返回错误码对应的默认安全文案
func defaultMessage(code string) string {
	switch code {
	case CodeInvalidArgument:
		return defaultInvalidArgumentMessage
	case CodeToolNotFound:
		return defaultToolNotFoundMessage
	case CodeExecution:
		return defaultExecutionMessage
	case CodeTimeout:
		return defaultTimeoutMessage
	case CodeCanceled:
		return defaultCanceledMessage
	case CodePanic:
		return defaultPanicMessage
	case CodeBadResponse:
		return defaultBadResponseMessage
	case CodeFillFailed:
		return defaultFillFailedMessage
	case CodeFillInvalid:
		return defaultFillInvalidMessage
	case CodeInputIncomplete:
		return defaultInputIncompleteMessage
	default:
		return defaultExecutionMessage
	}
}

// contextError 把 ctx 错误映射为超时或取消
func contextError(err error, toolName, callID string) *Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return newError(CodeTimeout, defaultTimeoutMessage, toolName, callID, err)
	}
	return newError(CodeCanceled, defaultCanceledMessage, toolName, callID, err)
}

// classifyHandlerError 把工具实现返回的错误封装为对外错误；底层原因只进 Cause
func classifyHandlerError(err error, toolName, callID string) *Error {
	if err == nil {
		return nil
	}
	// 工具嵌套调用时可能直接返回本包错误，保留其错误码与安全文案，原因摊平到最外层；
	// CodeToolNotFound 只由 Registry 产生，工具实现返回时按执行失败处理
	var existing *Error
	if errors.As(err, &existing) && existing != nil {
		code := existing.Code
		message := existing.SafeMessage()
		if !isKnownCode(code) || code == CodeToolNotFound {
			code = CodeExecution
			message = defaultExecutionMessage
		}
		out := newError(code, message, toolName, callID, existing.Cause())
		if code == CodeInputIncomplete {
			out.Missing = normalizeMissing(existing.Missing)
		}
		return out
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return newError(CodeTimeout, safeMessage(err, defaultTimeoutMessage), toolName, callID, err)
	case errors.Is(err, context.Canceled):
		return newError(CodeCanceled, safeMessage(err, defaultCanceledMessage), toolName, callID, err)
	case errors.Is(err, ErrInvalidArgument):
		return newError(CodeInvalidArgument, safeMessage(err, defaultInvalidArgumentMessage), toolName, callID, err)
	case errors.Is(err, ErrBadResponse):
		return newError(CodeBadResponse, safeMessage(err, defaultBadResponseMessage), toolName, callID, err)
	case errors.Is(err, ErrPanic):
		return newError(CodePanic, defaultPanicMessage, toolName, callID, err)
	case errors.Is(err, ErrInputIncomplete):
		return newIncompleteError(toolName, callID, nil)
	default:
		return newError(CodeExecution, safeMessage(err, defaultExecutionMessage), toolName, callID, err)
	}
}

// safeMessage 优先使用错误链上的 SafeError 文案，否则回退到默认文案
func safeMessage(err error, fallback string) string {
	var safeErr SafeError
	if errors.As(err, &safeErr) {
		if message := strings.TrimSpace(safeErr.SafeMessage()); message != "" {
			return truncateRunes(message, maxSafeMessageRunes)
		}
	}
	return fallback
}

// truncateRunes 按 rune 截断字符串，避免按字节截断产生非法 UTF-8
func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	count := 0
	for i := range text {
		if count == limit {
			return text[:i]
		}
		count++
	}
	return text
}
