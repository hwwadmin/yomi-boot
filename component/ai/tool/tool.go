// 定义工具接口与函数适配。
//
// @author zwei

package tool

import (
	"context"
	"encoding/json"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Tool 工具门面；Spec 返回定义，Invoke 执行一次调用。
// 实现须并发安全，定义在生命周期内保持稳定；注册表只在构造时快照一次定义。
type Tool interface {
	// Spec 返回工具定义；调用方不得修改返回值
	Spec() Spec
	// Invoke 执行一次工具调用
	//
	// call.Arguments 由 Registry.Invoke 归一为 JSON 对象；直接调用本方法时由调用方
	// 自行保证参数契约。返回内容须为合法 JSON，nil 等价于 null。
	Invoke(ctx context.Context, call Call) (json.RawMessage, error)
}

// Func 最简工具函数；参数为归一后的 JSON 对象，返回内容须为合法 JSON
type Func func(context.Context, json.RawMessage) (json.RawMessage, error)

// funcTool 用函数实现 Tool
type funcTool struct {
	spec Spec
	fn   Func
}

var _ Tool = (*funcTool)(nil)

// NewFunc 用函数构造 Tool，并校验工具定义；fn 为 nil 返回 CodeInvalidArgument
func NewFunc(spec Spec, fn Func) (Tool, error) {
	normalized, _, err := normalizeSpec(spec)
	if err != nil {
		return nil, err
	}
	return newFunc(normalized, fn)
}

// newFunc 用已规范化的定义构造函数工具
func newFunc(spec Spec, fn Func) (Tool, error) {
	if fn == nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, spec.Name, "", errors.New("ai/tool: 工具函数为 nil"))
	}
	return &funcTool{spec: spec, fn: fn}, nil
}

// Spec 实现 Tool；返回定义副本
func (t *funcTool) Spec() Spec {
	if t == nil {
		return Spec{}
	}
	return cloneSpec(t.spec)
}

// Invoke 实现 Tool；直接调用时参数契约由调用方负责
func (t *funcTool) Invoke(ctx context.Context, call Call) (json.RawMessage, error) {
	if t == nil {
		return nil, newError(CodeExecution, defaultExecutionMessage, "", call.ID, errors.New("ai/tool: 工具实现为空"))
	}
	if t.fn == nil {
		return nil, newError(CodeExecution, defaultExecutionMessage, t.spec.Name, call.ID, errors.New("ai/tool: 工具函数为 nil"))
	}
	return t.fn(ctx, call.Arguments)
}
