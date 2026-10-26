// 填槽流程测试。
//
// @author zwei

package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

type fakeCompleter struct {
	responses []string
	err       error
	calls     int
	requests  []CompletionRequest
}

func (f *fakeCompleter) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.requests = append(f.requests, req)
	index := f.calls
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if index >= len(f.responses) {
		return &CompletionResponse{}, nil
	}
	return &CompletionResponse{Content: f.responses[index]}, nil
}

func newFillRegistry(t *testing.T, completer Completer, fill FillConfig, tools ...Tool) *Registry {
	t.Helper()
	registry, err := New(Config{Tools: tools, Completer: completer, Fill: fill})
	if err != nil {
		t.Fatalf("构造注册表失败: %v", err)
	}
	return registry
}

func orderEvidence() *Evidence {
	return &Evidence{UserInput: "订单号 LPP3000456，商户单号 M-100"}
}

// TestFillAndInvokeResolved 覆盖完整填槽、typed 执行与 Prompt 内容
func TestFillAndInvokeResolved(t *testing.T) {
	t.Parallel()
	var normalizeCalls, handlerCalls atomic.Int32
	current := newOrderTool(t, &normalizeCalls, &handlerCalls)
	completer := &fakeCompleter{responses: []string{`{"order_id":"LPP3000456"}`}}
	registry := newFillRegistry(t, completer, FillConfig{}, current)

	filled, err := registry.Fill(context.Background(), Call{
		ID: "call-1", Name: "order_query", Evidence: orderEvidence(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !filled.Complete || filled.Source != FillSourceLLM || filled.Attempts != 1 {
		t.Fatalf("填槽结果不符: %+v", filled)
	}
	if filled.MissingPrompt != "请提供订单号或商户单号。" || filled.CallID != "call-1" || filled.Name != "order_query" {
		t.Fatalf("填槽上下文不符: %+v", filled)
	}
	if normalizeCalls.Load() != 1 {
		t.Fatalf("Normalize 应只执行一次: %d", normalizeCalls.Load())
	}
	in, ok := filled.Input.(fillOrderInput)
	if !ok || in.OrderID != "LPP3000456" {
		t.Fatalf("typed 入参不符: %+v", filled.Input)
	}
	if len(completer.requests) != 1 {
		t.Fatalf("模型调用次数不符: %d", len(completer.requests))
	}
	req := completer.requests[0]
	for _, section := range []string{"工具入参提取器", "## Schema", "## IdentifierKeys", "## Guidance", "## Example", "## Evidence"} {
		if !strings.Contains(req.System+req.User, section) {
			t.Fatalf("提示词缺少 %s: %s", section, req.User)
		}
	}
	if req.Tool != "order_query" || len(req.Schema) == 0 {
		t.Fatalf("CompletionRequest 不符: %+v", req)
	}

	result, err := registry.InvokeFilled(context.Background(), filled)
	if err != nil {
		t.Fatal(err)
	}
	if handlerCalls.Load() != 1 || normalizeCalls.Load() != 1 {
		t.Fatalf("InvokeFilled 应直接执行 typed 入参: handler=%d normalize=%d", handlerCalls.Load(), normalizeCalls.Load())
	}
	if result.CallID != "call-1" || string(result.Content) != `{"order_id":"LPP3000456"}` {
		t.Fatalf("执行结果不符: %+v", result)
	}
}

// TestFillMissingNoRetry 覆盖缺槽不重试、返回 Missing
func TestFillMissingNoRetry(t *testing.T) {
	t.Parallel()
	var handlerCalls atomic.Int32
	current := newOrderTool(t, nil, &handlerCalls)
	completer := &fakeCompleter{responses: []string{`{}`}}
	registry := newFillRegistry(t, completer, FillConfig{MaxAttempts: 3}, current)

	filled, err := registry.Fill(context.Background(), Call{
		ID: "call-1", Name: "order_query", Evidence: orderEvidence(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if filled.Complete || !sameStrings(filled.Missing, []string{"order_id", "merchant_order_no"}) {
		t.Fatalf("缺槽结果不符: %+v", filled)
	}
	if completer.calls != 1 {
		t.Fatalf("缺槽不应重试: %d", completer.calls)
	}
	if _, err := registry.InvokeFilled(context.Background(), filled); err == nil {
		t.Fatal("不完整结果不应执行")
	} else {
		toolErr := requireCode(t, err, CodeInputIncomplete)
		if !sameStrings(toolErr.Missing, filled.Missing) {
			t.Fatalf("Missing 未保留: %+v", toolErr)
		}
	}
	if handlerCalls.Load() != 0 {
		t.Fatal("缺槽时 handler 不应执行")
	}
}

// TestFillIdentifierGate 覆盖模型臆造标识符
func TestFillIdentifierGate(t *testing.T) {
	t.Parallel()
	completer := &fakeCompleter{responses: []string{`{"order_id":"FAKE-1"}`}}
	registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
	filled, err := registry.Fill(context.Background(), Call{
		ID: "call-1", Name: "order_query", Evidence: orderEvidence(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if filled.Complete || !sameStrings(filled.Missing, []string{"order_id"}) {
		t.Fatalf("标识符门闩不符: %+v", filled)
	}
	if completer.calls != 1 {
		t.Fatalf("标识符缺失不应重试: %d", completer.calls)
	}
}

// TestFillExtractVariants 覆盖 Markdown/前后缀/字符串花括号
func TestFillExtractVariants(t *testing.T) {
	t.Parallel()
	responses := []string{
		"```json\n{\"order_id\":\"LPP3000456\"}\n```",
		"好的，结果如下：\n{\"order_id\":\"LPP3000456\"}\n谢谢",
		`{"order_id":"LPP3000456","note":"含 } 花括号"}`,
	}
	for _, response := range responses {
		response := response
		t.Run(response[:min(12, len(response))], func(t *testing.T) {
			t.Parallel()
			completer := &fakeCompleter{responses: []string{response}}
			registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
			filled, err := registry.Fill(context.Background(), Call{
				ID: "call-1", Name: "order_query", Evidence: orderEvidence(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if !filled.Complete {
				t.Fatalf("抽取失败: %+v", filled)
			}
		})
	}
}

// TestFillRetryInvalid 覆盖非法 JSON 重试并带 Repair
func TestFillRetryInvalid(t *testing.T) {
	t.Parallel()
	completer := &fakeCompleter{responses: []string{
		"不是 JSON",
		`{"order_id":"LPP3000456"}`,
	}}
	registry := newFillRegistry(t, completer, FillConfig{MaxAttempts: 2}, newOrderTool(t, nil, nil))
	filled, err := registry.Fill(context.Background(), Call{
		ID: "call-1", Name: "order_query", Evidence: orderEvidence(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !filled.Complete || filled.Attempts != 2 || completer.calls != 2 {
		t.Fatalf("重试结果不符: %+v calls=%d", filled, completer.calls)
	}
	if !strings.Contains(completer.requests[1].User, "## Repair") ||
		!strings.Contains(completer.requests[1].User, "上次输出不是合法 JSON 对象") {
		t.Fatalf("重试提示缺少 Repair: %s", completer.requests[1].User)
	}
}

// TestFillRetryResolveError 覆盖类型错误重试
func TestFillRetryResolveError(t *testing.T) {
	t.Parallel()
	completer := &fakeCompleter{responses: []string{
		`{"order_id":123}`,
		`{"order_id":"LPP3000456"}`,
	}}
	registry := newFillRegistry(t, completer, FillConfig{MaxAttempts: 2}, newOrderTool(t, nil, nil))
	filled, err := registry.Fill(context.Background(), Call{
		ID: "call-1", Name: "order_query", Evidence: orderEvidence(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !filled.Complete || completer.calls != 2 {
		t.Fatalf("类型错误重试不符: %+v calls=%d", filled, completer.calls)
	}
	if !strings.Contains(completer.requests[1].User, "## Repair") {
		t.Fatalf("重试提示缺少 Repair: %s", completer.requests[1].User)
	}
}

// TestFillInvalidExhausted 覆盖重试耗尽
func TestFillInvalidExhausted(t *testing.T) {
	t.Parallel()
	completer := &fakeCompleter{responses: []string{"不是 JSON", "仍然不是 JSON"}}
	registry := newFillRegistry(t, completer, FillConfig{MaxAttempts: 2}, newOrderTool(t, nil, nil))
	filled, err := registry.Fill(context.Background(), Call{
		ID: "call-1", Name: "order_query", Evidence: orderEvidence(),
	})
	toolErr := requireCode(t, err, CodeFillInvalid)
	if filled == nil || filled.Complete || filled.Attempts != 2 || filled.Raw == "" {
		t.Fatalf("重试耗尽应返回审计结果: %+v", filled)
	}
	if !errors.Is(toolErr, ErrFillInvalid) || toolErr.Tool != "order_query" || toolErr.CallID != "call-1" {
		t.Fatalf("错误上下文不符: %+v", toolErr)
	}
}

// TestFillFailureModes 覆盖未配置、调用失败、空输出、未知工具与取消
func TestFillFailureModes(t *testing.T) {
	t.Parallel()
	t.Run("未配置 Completer", func(t *testing.T) {
		t.Parallel()
		registry := newRegistry(t, newOrderTool(t, nil, nil))
		_, err := registry.Fill(context.Background(), Call{Name: "order_query", Evidence: orderEvidence()})
		requireCode(t, err, CodeFillFailed)
	})
	t.Run("Completer 失败", func(t *testing.T) {
		t.Parallel()
		completer := &fakeCompleter{err: errors.New("upstream down")}
		registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
		_, err := registry.Fill(context.Background(), Call{Name: "order_query", Evidence: orderEvidence()})
		requireCode(t, err, CodeFillFailed)
	})
	t.Run("空输出", func(t *testing.T) {
		t.Parallel()
		completer := &fakeCompleter{responses: []string{"  "}}
		registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
		_, err := registry.Fill(context.Background(), Call{Name: "order_query", Evidence: orderEvidence()})
		requireCode(t, err, CodeFillInvalid)
	})
	t.Run("未知工具", func(t *testing.T) {
		t.Parallel()
		completer := &fakeCompleter{responses: []string{`{}`}}
		registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
		_, err := registry.Fill(context.Background(), Call{Name: "missing"})
		requireCode(t, err, CodeToolNotFound)
	})
	t.Run("空工具名", func(t *testing.T) {
		t.Parallel()
		completer := &fakeCompleter{responses: []string{`{}`}}
		registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
		_, err := registry.Fill(context.Background(), Call{Name: "  "})
		requireCode(t, err, CodeInvalidArgument)
	})
	t.Run("缺少证据", func(t *testing.T) {
		t.Parallel()
		completer := &fakeCompleter{responses: []string{`{"order_id":"LPP3000456"}`}}
		registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
		_, err := registry.Fill(context.Background(), Call{Name: "order_query"})
		requireCode(t, err, CodeFillFailed)
		if completer.calls != 0 {
			t.Fatal("缺少证据不应调用模型")
		}
	})
	t.Run("上下文取消", func(t *testing.T) {
		t.Parallel()
		completer := &fakeCompleter{responses: []string{`{}`}}
		registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := registry.Fill(ctx, Call{Name: "order_query", Evidence: orderEvidence()})
		requireCode(t, err, CodeCanceled)
		if completer.calls != 0 {
			t.Fatal("取消后不应调用模型")
		}
	})
	t.Run("上下文超时", func(t *testing.T) {
		t.Parallel()
		completer := &fakeCompleter{responses: []string{`{}`}}
		registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		_, err := registry.Fill(ctx, Call{Name: "order_query", Evidence: orderEvidence()})
		requireCode(t, err, CodeTimeout)
	})
	t.Run("nil 注册表", func(t *testing.T) {
		t.Parallel()
		var registry *Registry
		_, err := registry.Fill(context.Background(), Call{Name: "order_query"})
		requireCode(t, err, CodeInvalidArgument)
	})
}

// TestFillResponseTooLarge 覆盖模型输出超限
func TestFillResponseTooLarge(t *testing.T) {
	t.Parallel()
	completer := &fakeCompleter{responses: []string{`{"order_id":"LPP3000456"}`}}
	registry := newFillRegistry(t, completer, FillConfig{MaxResponseBytes: 8}, newOrderTool(t, nil, nil))
	filled, err := registry.Fill(context.Background(), Call{Name: "order_query", Evidence: orderEvidence()})
	requireCode(t, err, CodeFillInvalid)
	if filled == nil || filled.Complete {
		t.Fatalf("超限应返回审计结果: %+v", filled)
	}
}

// TestFillSimpleTool 覆盖 simple tool 的 Fill 与 raw 校验
func TestFillSimpleTool(t *testing.T) {
	t.Parallel()
	var handlerCalls atomic.Int32
	spec := validSpec("weather")
	spec.Fill = FillSpec{
		Required:       []string{"city"},
		IdentifierKeys: []string{"city"},
		Guidance:       "从原文提取城市",
		MissingPrompt:  "请提供城市。",
	}
	current := newTool(t, spec, func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		handlerCalls.Add(1)
		return args, nil
	})
	completer := &fakeCompleter{responses: []string{`{"city":"北京"}`}}
	registry := newFillRegistry(t, completer, FillConfig{}, current)
	filled, err := registry.Fill(context.Background(), Call{
		ID: "call-1", Name: "weather", Evidence: &Evidence{UserInput: "北京天气"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !filled.Complete || filled.Input != nil || string(filled.Arguments) != `{"city":"北京"}` {
		t.Fatalf("simple Fill 结果不符: %+v", filled)
	}
	result, err := registry.InvokeFilled(context.Background(), filled)
	if err != nil || handlerCalls.Load() != 1 || string(result.Content) != `{"city":"北京"}` {
		t.Fatalf("simple 执行不符: %+v %v calls=%d", result, err, handlerCalls.Load())
	}

	// 缺槽
	completer = &fakeCompleter{responses: []string{`{}`}}
	registry = newFillRegistry(t, completer, FillConfig{}, current)
	filled, err = registry.Fill(context.Background(), Call{Name: "weather", Evidence: &Evidence{UserInput: "北京天气"}})
	if err != nil || filled.Complete || !sameStrings(filled.Missing, []string{"city"}) {
		t.Fatalf("simple 缺槽不符: %+v %v", filled, err)
	}

	// 标识符不在证据中
	completer = &fakeCompleter{responses: []string{`{"city":"上海"}`}}
	registry = newFillRegistry(t, completer, FillConfig{}, current)
	filled, err = registry.Fill(context.Background(), Call{Name: "weather", Evidence: &Evidence{UserInput: "北京天气"}})
	if err != nil || filled.Complete || !sameStrings(filled.Missing, []string{"city"}) {
		t.Fatalf("simple 标识符门闩不符: %+v %v", filled, err)
	}
}

// TestInvokeSimpleFillable 覆盖 simple fillable 的 Invoke 缺槽语义
func TestInvokeSimpleFillable(t *testing.T) {
	t.Parallel()
	var handlerCalls atomic.Int32
	spec := validSpec("weather")
	spec.Fill = FillSpec{Required: []string{"city"}, IdentifierKeys: []string{"city"}}
	current := newTool(t, spec, func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		handlerCalls.Add(1)
		return args, nil
	})
	registry := newRegistry(t, current)

	_, err := registry.Invoke(context.Background(), Call{Name: "weather", Arguments: json.RawMessage(`{}`)})
	toolErr := requireCode(t, err, CodeInputIncomplete)
	if !sameStrings(toolErr.Missing, []string{"city"}) || handlerCalls.Load() != 0 {
		t.Fatalf("simple 缺槽不符: %+v", toolErr)
	}

	_, err = registry.Invoke(context.Background(), Call{
		Name: "weather", Arguments: json.RawMessage(`{"city":"上海"}`),
		Evidence: &Evidence{UserInput: "北京天气"},
	})
	toolErr = requireCode(t, err, CodeInputIncomplete)
	if !sameStrings(toolErr.Missing, []string{"city"}) || handlerCalls.Load() != 0 {
		t.Fatalf("simple 标识符门闩不符: %+v", toolErr)
	}

	if _, err := registry.Invoke(context.Background(), Call{Name: "weather", Arguments: json.RawMessage(`{"city":"北京"}`)}); err != nil {
		t.Fatal(err)
	}
	if handlerCalls.Load() != 1 {
		t.Fatalf("完整入参应执行: %d", handlerCalls.Load())
	}
}

// TestFillAndInvoke 覆盖组合入口
func TestFillAndInvoke(t *testing.T) {
	t.Parallel()
	var handlerCalls atomic.Int32
	current := newOrderTool(t, nil, &handlerCalls)
	completer := &fakeCompleter{responses: []string{`{"order_id":"LPP3000456"}`}}
	registry := newFillRegistry(t, completer, FillConfig{}, current)
	result, err := registry.FillAndInvoke(context.Background(), Call{
		ID: "call-1", Name: "order_query", Evidence: orderEvidence(),
	})
	if err != nil || handlerCalls.Load() != 1 || result.CallID != "call-1" {
		t.Fatalf("FillAndInvoke 不符: %+v %v calls=%d", result, err, handlerCalls.Load())
	}

	completer = &fakeCompleter{responses: []string{`{}`}}
	registry = newFillRegistry(t, completer, FillConfig{}, current)
	_, err = registry.FillAndInvoke(context.Background(), Call{Name: "order_query", Evidence: orderEvidence()})
	toolErr := requireCode(t, err, CodeInputIncomplete)
	if !sameStrings(toolErr.Missing, []string{"order_id", "merchant_order_no"}) || handlerCalls.Load() != 1 {
		t.Fatalf("FillAndInvoke 缺槽不符: %+v", toolErr)
	}
}

// TestFillConfigDefaultsAndValidation 覆盖 FillConfig 默认与非法值
func TestFillConfigDefaultsAndValidation(t *testing.T) {
	t.Parallel()
	registry, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if registry.fillConfig.MaxAttempts != DefaultMaxFillAttempts ||
		registry.fillConfig.MaxHistoryTurns != DefaultMaxFillHistoryTurns ||
		registry.fillConfig.MaxHistoryRunes != DefaultMaxFillHistoryRunes ||
		registry.fillConfig.MaxResponseBytes != DefaultMaxFillResponseBytes {
		t.Fatalf("FillConfig 默认值不符: %+v", registry.fillConfig)
	}
	cases := []FillConfig{
		{MaxAttempts: -1},
		{MaxHistoryTurns: -1},
		{MaxHistoryRunes: -1},
		{MaxResponseBytes: -1},
	}
	for _, cfg := range cases {
		cfg := cfg
		if _, err := New(Config{Fill: cfg}); err == nil {
			t.Fatalf("非法 FillConfig 应被拒: %+v", cfg)
		} else {
			requireCode(t, err, CodeInvalidArgument)
		}
	}
}

type fillUpstreamError struct {
	text string
}

func (e *fillUpstreamError) Error() string { return e.text }

type panickingCompleter struct{}

func (panickingCompleter) Complete(context.Context, CompletionRequest) (*CompletionResponse, error) {
	panic("completer panic detail")
}

// TestFillCompleterPanic 覆盖自定义 Completer panic 兜底
func TestFillCompleterPanic(t *testing.T) {
	t.Parallel()
	registry := newFillRegistry(t, panickingCompleter{}, FillConfig{}, newOrderTool(t, nil, nil))
	_, err := registry.Fill(context.Background(), Call{Name: "order_query", Evidence: orderEvidence()})
	toolErr := requireCode(t, err, CodeFillFailed)
	if strings.Contains(toolErr.Error(), "completer panic detail") ||
		toolErr.Cause() == nil || !strings.Contains(toolErr.Cause().Error(), "completer panic detail") {
		t.Fatalf("Completer panic 封装不符: %+v", toolErr)
	}
}

// TestFillErrorEncapsulation 覆盖填槽错误不暴露底层
func TestFillErrorEncapsulation(t *testing.T) {
	t.Parallel()
	upstream := &fillUpstreamError{text: "secret upstream detail"}
	completer := &fakeCompleter{err: upstream}
	registry := newFillRegistry(t, completer, FillConfig{}, newOrderTool(t, nil, nil))
	_, err := registry.Fill(context.Background(), Call{Name: "order_query", Evidence: orderEvidence()})
	toolErr := requireCode(t, err, CodeFillFailed)
	if strings.Contains(toolErr.Error(), upstream.text) {
		t.Fatal("Error() 不应包含底层错误")
	}
	var target *fillUpstreamError
	if errors.As(err, &target) {
		t.Fatal("不应能 As 到底层错误")
	}
	if toolErr.Cause() == nil || !strings.Contains(toolErr.Cause().Error(), upstream.text) {
		t.Fatal("Cause 应保留底层错误")
	}
	serialized, marshalErr := json.Marshal(toolErr)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(serialized), upstream.text) {
		t.Fatal("JSON 不应包含底层错误")
	}
	if !errors.Is(err, ErrFillFailed) || errors.Is(err, ErrExecution) {
		t.Fatal("Fill 错误分类不符")
	}
}

// TestIncompleteErrorShape 覆盖缺槽错误结构与去重
func TestIncompleteErrorShape(t *testing.T) {
	t.Parallel()
	toolErr := newIncompleteError("order_query", "call-1", []string{"b", "a", "a", " "})
	if !sameStrings(toolErr.Missing, []string{"b", "a"}) {
		t.Fatalf("Missing 去重不符: %+v", toolErr.Missing)
	}
	if !errors.Is(toolErr, ErrInputIncomplete) || errors.Is(toolErr, ErrExecution) {
		t.Fatal("缺槽错误分类不符")
	}
	serialized, err := json.Marshal(toolErr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(serialized), `"missing":["b","a"]`) {
		t.Fatalf("Missing 未进入 JSON: %s", serialized)
	}
}
