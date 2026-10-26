// 定义 typed 入参契约、解析与 resolved 工具。
//
// @author zwei

package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// InputContract 定义模型 JSON 到工具 typed 入参的解析契约
type InputContract[In any] struct {
	// Decode 把模型 JSON 解成 typed 入参；nil 时使用 json.Unmarshal
	Decode func(raw json.RawMessage, ev *Evidence) (In, error)
	// Normalize 做 trim、别名、默认值与派生字段；nil 时不处理
	Normalize func(In) (In, error)
	// Validate 做业务必填、互斥与条件校验；返回缺失槽位键，error 表示硬失败
	Validate func(In) (missing []string, err error)
	// Identify 返回标识符键值；nil 时按 JSON 字段名从 In 编码结果中取字符串
	Identify func(In) map[string]string
}

// Resolution 一次入参解析结果；缺槽时 Complete 为 false，错误另行返回
type Resolution struct {
	Input    any      `json:"-"`                 // Input 是解析后的 typed 入参
	Missing  []string `json:"missing,omitempty"` // Missing 是缺失槽位键
	Complete bool     `json:"complete"`          // Complete 表示是否完整
}

// InputResolver 由支持 typed 入参解析的工具实现
type InputResolver interface {
	// Resolve 把模型 JSON 解析为 typed 入参；缺槽返回 Resolution，硬错误返回 error
	Resolve(raw json.RawMessage, ev *Evidence) (*Resolution, error)
}

// ResolvedTool 由支持直接执行 typed 入参的工具实现
type ResolvedTool interface {
	Tool
	// InvokeResolved 直接执行已经解析好的 typed 入参
	InvokeResolved(ctx context.Context, input any) (json.RawMessage, error)
}

// Resolve 解析工具入参；缺槽通过 Resolution 返回，不返回错误
func Resolve[In any](raw json.RawMessage, spec Spec, contract InputContract[In], ev *Evidence) (*Resolution, error) {
	normalized, schemaRequired, err := normalizeSpec(spec)
	if err != nil {
		return nil, err
	}
	if normalized.Fill.EvidenceOnly && contract.Decode == nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, normalized.Name, "",
			errors.New("ai/tool: EvidenceOnly 需要自定义 Decode"))
	}
	return resolveWithSpec(normalized, schemaRequired, contract, raw, ev)
}

// resolveWithSpec 使用已规范化的工具定义解析入参
func resolveWithSpec[In any](
	spec Spec,
	schemaRequired []string,
	contract InputContract[In],
	raw json.RawMessage,
	ev *Evidence,
) (*Resolution, error) {
	arguments, _, err := normalizeArguments(raw, 0, spec.Name, "")
	if err != nil {
		return nil, err
	}
	var input In
	if contract.Decode != nil {
		input, err = contract.Decode(arguments, ev)
	} else {
		err = json.Unmarshal(arguments, &input)
	}
	if err != nil {
		return nil, invalidInputError(err, spec.Name, defaultInvalidArgumentMessage)
	}
	if contract.Normalize != nil {
		input, err = contract.Normalize(input)
		if err != nil {
			return nil, invalidInputError(err, spec.Name, defaultInvalidArgumentMessage)
		}
	}
	var missing []string
	if contract.Validate != nil {
		current, err := contract.Validate(input)
		if err != nil {
			return nil, invalidInputError(err, spec.Name, defaultInvalidArgumentMessage)
		}
		missing = append(missing, current...)
	} else {
		keys := spec.Fill.Required
		if len(keys) == 0 {
			keys = schemaRequired
		}
		current, err := requiredMissingFromValue(input, keys)
		if err != nil {
			return nil, invalidInputError(err, spec.Name, defaultInvalidArgumentMessage)
		}
		missing = append(missing, current...)
	}
	if ev != nil && len(spec.Fill.IdentifierKeys) > 0 {
		values, err := identifyValues(input, spec.Fill.IdentifierKeys, contract.Identify)
		if err != nil {
			return nil, invalidInputError(err, spec.Name, defaultInvalidArgumentMessage)
		}
		missing = append(missing, identifierMissing(values, spec.Fill.IdentifierKeys, ev)...)
	}
	missing = normalizeMissing(missing)
	return &Resolution{Input: input, Missing: missing, Complete: len(missing) == 0}, nil
}

// NewResolved 构造带 typed 入参解析的工具
func NewResolved[In, Out any](
	spec Spec,
	contract InputContract[In],
	fn func(context.Context, In) (Out, error),
) (Tool, error) {
	normalized, schemaRequired, err := normalizeSpec(spec)
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, normalized.Name, "", errors.New("ai/tool: 工具函数为 nil"))
	}
	if normalized.Fill.EvidenceOnly && contract.Decode == nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, normalized.Name, "", errors.New("ai/tool: EvidenceOnly 需要自定义 Decode"))
	}
	tool := &resolvedTool{
		spec: normalized,
		resolve: func(raw json.RawMessage, ev *Evidence) (*Resolution, error) {
			return resolveWithSpec(normalized, schemaRequired, contract, raw, ev)
		},
		invoke: func(ctx context.Context, input any) (json.RawMessage, error) {
			in, ok := input.(In)
			if !ok {
				return nil, newError(CodeExecution, defaultExecutionMessage, normalized.Name, "", errors.New("ai/tool: 内部入参类型不匹配"))
			}
			out, err := fn(ctx, in)
			if err != nil {
				return nil, err
			}
			content, err := marshalSafe(out)
			if err != nil {
				return nil, newError(CodeBadResponse, defaultBadResponseMessage, normalized.Name, "", err)
			}
			return content, nil
		},
	}
	return tool, nil
}

// resolvedTool 是 NewResolved 的实现；构造后不可变
type resolvedTool struct {
	spec    Spec
	resolve func(raw json.RawMessage, ev *Evidence) (*Resolution, error)
	invoke  func(ctx context.Context, input any) (json.RawMessage, error)
}

var (
	_ Tool          = (*resolvedTool)(nil)
	_ InputResolver = (*resolvedTool)(nil)
	_ ResolvedTool  = (*resolvedTool)(nil)
)

// Spec 实现 Tool；返回定义副本
func (t *resolvedTool) Spec() Spec {
	if t == nil {
		return Spec{}
	}
	return cloneSpec(t.spec)
}

// Resolve 实现 InputResolver
func (t *resolvedTool) Resolve(raw json.RawMessage, ev *Evidence) (*Resolution, error) {
	if t == nil {
		return nil, newError(CodeExecution, defaultExecutionMessage, "", "", errors.New("ai/tool: 工具实现为空"))
	}
	return t.resolve(raw, ev)
}

// Invoke 实现 Tool；先解析 typed 入参，缺槽返回 ErrInputIncomplete
func (t *resolvedTool) Invoke(ctx context.Context, call Call) (json.RawMessage, error) {
	if t == nil {
		return nil, newError(CodeExecution, defaultExecutionMessage, "", call.ID, errors.New("ai/tool: 工具实现为空"))
	}
	resolution, err := t.resolve(call.Arguments, call.Evidence)
	if err != nil {
		return nil, withCallContext(err, t.spec.Name, call.ID)
	}
	if !resolution.Complete {
		return nil, newIncompleteError(t.spec.Name, call.ID, resolution.Missing)
	}
	return t.invoke(ctx, resolution.Input)
}

// InvokeResolved 实现 ResolvedTool；直接执行 typed 入参
func (t *resolvedTool) InvokeResolved(ctx context.Context, input any) (json.RawMessage, error) {
	if t == nil {
		return nil, newError(CodeExecution, defaultExecutionMessage, "", "", errors.New("ai/tool: 工具实现为空"))
	}
	return t.invoke(ctx, input)
}

// invalidInputError 将解析错误包装为入参错误；已是本包错误时保留错误码与安全文案
func invalidInputError(err error, toolName, fallback string) *Error {
	if err == nil {
		return nil
	}
	var existing *Error
	if errors.As(err, &existing) && existing != nil {
		return withCallContext(existing, toolName, "")
	}
	return newError(CodeInvalidArgument, safeMessage(err, fallback), toolName, "", err)
}

// requiredMissingFromValue 按 JSON 字段名检查 typed 入参的必填键
func requiredMissingFromValue[In any](input In, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	raw, err := marshalSafe(input)
	if err != nil {
		return nil, errors.Wrap(err, "ai/tool: 入参编码失败")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, errors.Wrap(err, "ai/tool: 入参不是 JSON 对象")
	}
	return rawRequiredMissing(object, keys), nil
}

// identifyValues 取标识符值；未提供 Identify 时按 JSON 字段名取字符串
func identifyValues[In any](input In, keys []string, identify func(In) map[string]string) (map[string]string, error) {
	if identify != nil {
		return identify(input), nil
	}
	raw, err := marshalSafe(input)
	if err != nil {
		return nil, errors.Wrap(err, "ai/tool: 标识符取值失败")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, errors.Wrap(err, "ai/tool: 标识符入参不是 JSON 对象")
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		rawValue, ok := object[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			continue
		}
		values[key] = value
	}
	return values, nil
}

// rawRequiredMissing 检查原始 JSON 对象的必填键；null 与空字符串视为缺失
func rawRequiredMissing(object map[string]json.RawMessage, keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	var missing []string
	for _, key := range keys {
		rawValue, ok := object[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		trimmed := bytes.TrimSpace(rawValue)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			missing = append(missing, key)
			continue
		}
		var value string
		if err := json.Unmarshal(trimmed, &value); err == nil && strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

// rawIdentifierMissing 检查原始 JSON 对象中的标识符是否出现在证据原文
func rawIdentifierMissing(object map[string]json.RawMessage, keys []string, ev *Evidence) []string {
	if ev == nil || len(keys) == 0 {
		return nil
	}
	blob := evidenceBlob(ev)
	var missing []string
	for _, key := range keys {
		rawValue, ok := object[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !containsFold(blob, value) {
			missing = append(missing, key)
		}
	}
	return missing
}

// identifierMissing 检查标识符值是否出现在证据原文；空值跳过
func identifierMissing(values map[string]string, keys []string, ev *Evidence) []string {
	if ev == nil || len(keys) == 0 {
		return nil
	}
	blob := evidenceBlob(ev)
	var missing []string
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		if !containsFold(blob, value) {
			missing = append(missing, key)
		}
	}
	return missing
}

// evidenceBlob 拼接证据正文，用于标识符子串匹配
func evidenceBlob(ev *Evidence) string {
	if ev == nil {
		return ""
	}
	parts := make([]string, 0, 2+len(ev.OCR)+len(ev.History))
	if strings.TrimSpace(ev.UserInput) != "" {
		parts = append(parts, ev.UserInput)
	}
	for _, text := range ev.OCR {
		if strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	for _, turn := range ev.History {
		if strings.TrimSpace(turn.Content) != "" {
			parts = append(parts, turn.Content)
		}
	}
	return strings.Join(parts, "\n")
}

// containsFold 大小写不敏感子串匹配
func containsFold(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
