// 定义泛型工具适配与参数解码。
//
// @author zwei

package tool

import (
	"context"
	"encoding/json"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// NewTyped 用泛型函数构造 Tool：参数解码为 In，返回值编码为 JSON。
// 参数只做对象形态与类型解码；根级 required 由 Registry.Invoke 校验，嵌套业务校验由 In 自行负责。
func NewTyped[In, Out any](spec Spec, fn func(context.Context, In) (Out, error)) (Tool, error) {
	normalized, _, err := normalizeSpec(spec)
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, normalized.Name, "", errors.New("ai/tool: 工具函数为 nil"))
	}
	return newFunc(normalized, func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		in, err := DecodeArgs[In](args)
		if err != nil {
			return nil, err
		}
		out, err := fn(ctx, in)
		if err != nil {
			return nil, err
		}
		content, err := json.Marshal(out)
		if err != nil {
			return nil, newError(CodeBadResponse, defaultBadResponseMessage, normalized.Name, "", err)
		}
		return content, nil
	})
}

// DecodeArgs 把归一后的参数解码到 T；空或 null 视为 {}，非对象或类型不匹配返回 CodeInvalidArgument。
// 未知字段按 encoding/json 默认行为忽略；需要严格拒绝时使用 NewFunc 自定义 decoder。
func DecodeArgs[T any](args json.RawMessage) (T, error) {
	var out T
	normalized, _, err := normalizeArguments(args, 0, "", "")
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(normalized, &out); err != nil {
		return out, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, "", "", err)
	}
	return out, nil
}
