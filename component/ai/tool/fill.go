// 定义填槽配置、结果与 Registry 填槽流程。
//
// @author zwei

package tool

import (
	"context"
	"encoding/json"
	"runtime/debug"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	// DefaultMaxFillAttempts 是填槽默认尝试次数
	DefaultMaxFillAttempts = 1
	// DefaultMaxFillHistoryTurns 是填槽默认保留的历史轮数
	DefaultMaxFillHistoryTurns = 6
	// DefaultMaxFillHistoryRunes 是单条历史默认保留的字符数
	DefaultMaxFillHistoryRunes = 500
	// DefaultMaxFillResponseBytes 是模型输出默认上限
	DefaultMaxFillResponseBytes = 1 << 20

	// maxFillRawRunes 是 Filled.Raw 审计保留的最大字符数
	maxFillRawRunes = 8192
)

// FillConfig 填槽配置；本包不读 YAML
type FillConfig struct {
	MaxAttempts      int // MaxAttempts 是最大尝试次数；0 用默认值 1，负数非法
	MaxHistoryTurns  int // MaxHistoryTurns 是提示词保留的历史轮数；0 用默认值 6
	MaxHistoryRunes  int // MaxHistoryRunes 是单条历史字符上限；0 用默认值 500
	MaxResponseBytes int // MaxResponseBytes 是模型输出字节上限；0 用默认值 1 MiB
}

// FillSource 填槽来源
type FillSource string

const (
	// FillSourceLLM 表示由模型填槽
	FillSourceLLM FillSource = "llm"
	// FillSourceEvidence 表示由证据确定性映射
	FillSourceEvidence FillSource = "evidence"
)

// Filled 一次填槽结果；Complete 为 false 时业务应追问用户，不得执行工具
type Filled struct {
	CallID        string          `json:"call_id,omitempty"`        // CallID 回指 Call.ID
	Name          string          `json:"name"`                     // Name 是规范工具名
	Arguments     json.RawMessage `json:"arguments"`                // Arguments 是模型生成/证据映射的 JSON 对象
	Input         any             `json:"-"`                        // Input 是 typed 入参；simple tool 为 nil
	Missing       []string        `json:"missing,omitempty"`        // Missing 是缺失槽位键
	MissingPrompt string          `json:"missing_prompt,omitempty"` // MissingPrompt 是缺槽时的默认追问文案
	Complete      bool            `json:"complete"`                 // Complete 表示入参是否完整
	Source        FillSource      `json:"source"`                   // Source 是填槽来源
	Attempts      int             `json:"attempts"`                 // Attempts 是模型尝试次数；证据映射为 0
	Raw           string          `json:"-"`                        // Raw 是截断后的模型原始输出，仅审计用，禁止对外
}

// Fill 调用模型按工具契约生成入参 JSON；缺槽返回 Filled，不返回错误
func (r *Registry) Fill(ctx context.Context, call Call) (*Filled, error) {
	name := strings.TrimSpace(call.Name)
	if r == nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidConfigMessage, name, call.ID, errors.New("ai/tool: 注册表未初始化"))
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(err, name, call.ID)
	}
	if name == "" {
		return nil, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, "", call.ID, errors.New("ai/tool: 工具名为空"))
	}
	current, ok := r.entries[name]
	if !ok {
		return nil, newError(CodeToolNotFound, defaultToolNotFoundMessage, name, call.ID, errors.Newf("ai/tool: 工具 %q 未注册", name))
	}
	if (len(current.spec.Fill.IdentifierKeys) > 0 || current.spec.Fill.EvidenceOnly) && call.Evidence == nil {
		return nil, newError(CodeFillFailed, defaultFillFailedMessage, current.spec.Name, call.ID,
			errors.New("ai/tool: 缺少填槽证据"))
	}
	if current.spec.Fill.EvidenceOnly {
		return r.fillFromEvidence(current, call)
	}
	if r.completer == nil {
		return nil, newError(CodeFillFailed, defaultFillFailedMessage, current.spec.Name, call.ID, errors.New("ai/tool: 未配置 Completer"))
	}
	attempts := r.fillAttempts()
	maxResponseBytes := r.fillResponseLimit()
	var (
		lastRaw       string
		lastArguments json.RawMessage
		lastErr       error
		repair        *FillRepair
	)
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, contextError(err, current.spec.Name, call.ID)
		}
		system, user := BuildFillPrompt(current.spec, call.Evidence, r.fillConfig, repair)
		response, err := completeSafe(ctx, r.completer, CompletionRequest{
			Tool:   current.spec.Name,
			System: system,
			User:   user,
			Schema: current.spec.Parameters,
		})
		if err != nil {
			return nil, newError(CodeFillFailed, defaultFillFailedMessage, current.spec.Name, call.ID,
				errors.Wrap(err, "ai/tool: 填槽模型调用失败"))
		}
		if response == nil || strings.TrimSpace(response.Content) == "" {
			lastRaw = ""
			lastArguments = nil
			lastErr = errors.New("ai/tool: 模型输出为空")
			if attempt < attempts {
				repair = &FillRepair{Problem: "上次输出为空"}
				continue
			}
			break
		}
		raw := response.Content
		lastRaw = raw
		if len(raw) > maxResponseBytes {
			lastArguments = nil
			lastErr = errors.Newf("ai/tool: 模型输出 %d 字节超过上限 %d", len(raw), maxResponseBytes)
			if attempt < attempts {
				repair = &FillRepair{Previous: raw, Problem: "上次输出超过大小上限"}
				continue
			}
			break
		}
		object, err := extractJSONObject(raw)
		if err != nil {
			lastArguments = nil
			lastErr = err
			if attempt < attempts {
				repair = &FillRepair{Previous: raw, Problem: "上次输出不是合法 JSON 对象"}
				continue
			}
			break
		}
		if len(object) > r.argumentsLimit() {
			lastArguments = nil
			lastErr = errors.Newf("ai/tool: 参数 %d 字节超过上限 %d", len(object), r.argumentsLimit())
			if attempt < attempts {
				repair = &FillRepair{Previous: raw, Problem: "上次输出参数超过大小上限"}
				continue
			}
			break
		}
		lastArguments = object
		resolution, err := r.resolveFill(current, object, call)
		if err != nil {
			lastErr = err
			if attempt < attempts {
				repair = &FillRepair{Previous: raw, Problem: fillProblem(err)}
				continue
			}
			break
		}
		if !resolution.Complete {
			return &Filled{
				CallID:        call.ID,
				Name:          current.spec.Name,
				Arguments:     object,
				Input:         resolution.Input,
				Missing:       resolution.Missing,
				MissingPrompt: current.spec.Fill.MissingPrompt,
				Complete:      false,
				Source:        FillSourceLLM,
				Attempts:      attempt,
				Raw:           truncateRunes(raw, maxFillRawRunes),
			}, nil
		}
		return &Filled{
			CallID:        call.ID,
			Name:          current.spec.Name,
			Arguments:     object,
			Input:         resolution.Input,
			MissingPrompt: current.spec.Fill.MissingPrompt,
			Complete:      true,
			Source:        FillSourceLLM,
			Attempts:      attempt,
			Raw:           truncateRunes(raw, maxFillRawRunes),
		}, nil
	}
	cause := fillCause(lastErr)
	if cause == nil {
		cause = lastErr
	}
	filled := &Filled{
		CallID:        call.ID,
		Name:          current.spec.Name,
		Arguments:     lastArguments,
		MissingPrompt: current.spec.Fill.MissingPrompt,
		Complete:      false,
		Source:        FillSourceLLM,
		Attempts:      attempts,
		Raw:           truncateRunes(lastRaw, maxFillRawRunes),
	}
	return filled, newError(CodeFillInvalid, defaultFillInvalidMessage, current.spec.Name, call.ID, cause)
}

// fillFromEvidence 跳过模型，直接用证据映射 typed 入参
func (r *Registry) fillFromEvidence(current *entry, call Call) (*Filled, error) {
	if current.resolver == nil {
		return nil, newError(CodeFillInvalid, defaultFillInvalidMessage, current.spec.Name, call.ID,
			errors.New("ai/tool: EvidenceOnly 需要 InputResolver"))
	}
	resolution, err := resolveSafe(current.resolver, nil, call.Evidence)
	if err != nil {
		return nil, newError(CodeFillInvalid, fillProblem(err), current.spec.Name, call.ID, fillCause(err))
	}
	if resolution == nil {
		return nil, newError(CodeFillInvalid, defaultFillInvalidMessage, current.spec.Name, call.ID,
			errors.New("ai/tool: 证据映射没有返回结果"))
	}
	resolution.Missing = normalizeMissing(resolution.Missing)
	resolution.Complete = len(resolution.Missing) == 0
	arguments, err := marshalSafe(resolution.Input)
	if err != nil || len(arguments) == 0 || arguments[0] != '{' {
		cause := err
		if cause == nil {
			cause = errors.New("ai/tool: 证据映射结果不是 JSON 对象")
		}
		return nil, newError(CodeFillInvalid, defaultFillInvalidMessage, current.spec.Name, call.ID, cause)
	}
	if len(arguments) > r.argumentsLimit() {
		return nil, newError(CodeFillInvalid, defaultArgumentsTooLargeMessage, current.spec.Name, call.ID,
			errors.Newf("ai/tool: 证据映射参数 %d 字节超过上限 %d", len(arguments), r.argumentsLimit()))
	}
	return &Filled{
		CallID:        call.ID,
		Name:          current.spec.Name,
		Arguments:     arguments,
		Input:         resolution.Input,
		Missing:       resolution.Missing,
		MissingPrompt: current.spec.Fill.MissingPrompt,
		Complete:      resolution.Complete,
		Source:        FillSourceEvidence,
		Attempts:      0,
	}, nil
}

// resolveFill 按工具是否实现 InputResolver 选择 typed 或 raw 校验
func (r *Registry) resolveFill(current *entry, arguments json.RawMessage, call Call) (*Resolution, error) {
	if current.resolver != nil {
		resolution, err := resolveSafe(current.resolver, arguments, call.Evidence)
		if err != nil {
			return nil, err
		}
		if resolution == nil {
			return nil, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, current.spec.Name, call.ID,
				errors.New("ai/tool: 入参解析器返回空结果"))
		}
		resolution.Missing = normalizeMissing(resolution.Missing)
		resolution.Complete = len(resolution.Missing) == 0
		return resolution, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &object); err != nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, current.spec.Name, call.ID,
			errors.Wrap(err, "ai/tool: 参数不是合法 JSON 对象"))
	}
	missing := rawRequiredMissing(object, current.fillRequired)
	missing = append(missing, rawIdentifierMissing(object, current.spec.Fill.IdentifierKeys, call.Evidence)...)
	missing = normalizeMissing(missing)
	return &Resolution{Missing: missing, Complete: len(missing) == 0}, nil
}

// InvokeFilled 执行已完成的填槽结果；不完整结果不会执行工具
func (r *Registry) InvokeFilled(ctx context.Context, filled *Filled) (*Result, error) {
	if filled == nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, "", "", errors.New("ai/tool: 填槽结果为空"))
	}
	name := strings.TrimSpace(filled.Name)
	if r == nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidConfigMessage, name, filled.CallID, errors.New("ai/tool: 注册表未初始化"))
	}
	if name == "" {
		return nil, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, "", filled.CallID, errors.New("ai/tool: 工具名为空"))
	}
	current, ok := r.entries[name]
	if !ok {
		return nil, newError(CodeToolNotFound, defaultToolNotFoundMessage, name, filled.CallID, errors.Newf("ai/tool: 工具 %q 未注册", name))
	}
	if !filled.Complete {
		return nil, newIncompleteError(current.spec.Name, filled.CallID, filled.Missing)
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(err, current.spec.Name, filled.CallID)
	}
	if filled.Input != nil && current.resolved != nil {
		content, err := invokeResolvedTool(ctx, current.resolved, filled.Input, current.spec.Name, filled.CallID)
		if err != nil {
			return nil, err
		}
		content, err = normalizeContent(content, r.contentLimit(), current.spec.Name, filled.CallID)
		if err != nil {
			return nil, err
		}
		return &Result{CallID: filled.CallID, Name: current.spec.Name, Content: content}, nil
	}
	if filled.Input == nil && current.resolver != nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, current.spec.Name, filled.CallID,
			errors.New("ai/tool: 已解析工具缺少 typed 入参"))
	}
	return r.Invoke(ctx, Call{ID: filled.CallID, Name: current.spec.Name, Arguments: filled.Arguments})
}

// FillAndInvoke 先填槽再执行；缺槽返回 ErrInputIncomplete
func (r *Registry) FillAndInvoke(ctx context.Context, call Call) (*Result, error) {
	filled, err := r.Fill(ctx, call)
	if err != nil {
		return nil, err
	}
	if !filled.Complete {
		return nil, newIncompleteError(filled.Name, filled.CallID, filled.Missing)
	}
	return r.InvokeFilled(ctx, filled)
}

// invokeResolvedTool 执行 typed 入参并兜底 panic
func invokeResolvedTool(ctx context.Context, current ResolvedTool, input any, toolName, callID string) (content json.RawMessage, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			content = nil
			err = newError(CodePanic, defaultPanicMessage, toolName, callID,
				errors.Newf("ai/tool: 工具执行 panic: %v\n%s", recovered, debug.Stack()))
		}
	}()
	content, err = current.InvokeResolved(ctx, input)
	if err != nil {
		return nil, classifyHandlerError(err, toolName, callID)
	}
	return content, nil
}

// completeSafe 调用填槽模型并兜底 panic，避免自定义 Completer 打崩进程
func completeSafe(ctx context.Context, completer Completer, req CompletionRequest) (response *CompletionResponse, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			response = nil
			err = errors.Newf("ai/tool: 填槽模型 panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	return completer.Complete(ctx, req)
}

// resolveSafe 调用入参解析器并兜底 panic
func resolveSafe(resolver InputResolver, raw json.RawMessage, ev *Evidence) (resolution *Resolution, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			resolution = nil
			err = errors.Newf("ai/tool: 入参解析 panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	return resolver.Resolve(raw, ev)
}

// marshalSafe 编码 JSON 并兜底自定义 MarshalJSON panic
func marshalSafe(value any) (raw []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			raw = nil
			err = errors.Newf("ai/tool: JSON 编码 panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	return json.Marshal(value)
}

// fillProblem 返回可放入重试提示的安全问题描述
func fillProblem(err error) string {
	var current *Error
	if errors.As(err, &current) && current != nil && current.SafeMessage() != "" {
		return current.SafeMessage()
	}
	return "上次输出不符合工具入参契约"
}

// fillCause 取底层原因；本包错误摊平到 Cause
func fillCause(err error) error {
	var current *Error
	if errors.As(err, &current) && current != nil {
		return current.Cause()
	}
	return err
}

// fillAttempts 返回生效的填槽尝试次数
func (r *Registry) fillAttempts() int {
	if r == nil || r.fillConfig.MaxAttempts <= 0 {
		return DefaultMaxFillAttempts
	}
	return r.fillConfig.MaxAttempts
}

// fillResponseLimit 返回生效的模型输出上限
func (r *Registry) fillResponseLimit() int {
	if r == nil || r.fillConfig.MaxResponseBytes <= 0 {
		return DefaultMaxFillResponseBytes
	}
	return r.fillConfig.MaxResponseBytes
}
