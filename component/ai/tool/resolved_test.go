// typed 入参契约与 resolved 工具测试。
//
// @author zwei

package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

type fillOrderInput struct {
	OrderID         string `json:"order_id"`
	MerchantOrderNo string `json:"merchant_order_no"`
	NotCreated      bool   `json:"not_created"`
}

type fillOrderOutput struct {
	OrderID string `json:"order_id"`
}

func fillOrderSpec() Spec {
	return Spec{
		Name:        "order_query",
		Description: "查询订单状态与失败原因",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"order_id": {"type": "string"},
				"merchant_order_no": {"type": "string"},
				"not_created": {"type": "boolean"}
			}
		}`),
		Fill: FillSpec{
			Required:       []string{"order_id", "merchant_order_no"},
			IdentifierKeys: []string{"order_id", "merchant_order_no"},
			Guidance:       "order_id 与 merchant_order_no 至少提取其一，值须逐字来自原文",
			Example:        `{"order_id":"ORD-1"}`,
			MissingPrompt:  "请提供订单号或商户单号。",
		},
	}
}

func fillOrderContract(normalizeCalls *atomic.Int32) InputContract[fillOrderInput] {
	return InputContract[fillOrderInput]{
		Normalize: func(in fillOrderInput) (fillOrderInput, error) {
			if normalizeCalls != nil {
				normalizeCalls.Add(1)
			}
			in.OrderID = strings.TrimSpace(in.OrderID)
			in.MerchantOrderNo = strings.TrimSpace(in.MerchantOrderNo)
			return in, nil
		},
		Validate: func(in fillOrderInput) ([]string, error) {
			if in.OrderID == "" && in.MerchantOrderNo == "" {
				return []string{"order_id", "merchant_order_no"}, nil
			}
			return nil, nil
		},
	}
}

func newOrderTool(t *testing.T, normalizeCalls *atomic.Int32, handlerCalls *atomic.Int32) Tool {
	t.Helper()
	current, err := NewResolved(fillOrderSpec(), fillOrderContract(normalizeCalls),
		func(_ context.Context, in fillOrderInput) (fillOrderOutput, error) {
			if handlerCalls != nil {
				handlerCalls.Add(1)
			}
			return fillOrderOutput{OrderID: in.OrderID}, nil
		})
	if err != nil {
		t.Fatalf("构造 resolved 工具失败: %v", err)
	}
	return current
}

// TestResolveOrderInput 覆盖 Decode/Normalize/Validate/IdentifierGate
func TestResolveOrderInput(t *testing.T) {
	t.Parallel()
	spec := fillOrderSpec()
	contract := fillOrderContract(nil)
	cases := []struct {
		name       string
		raw        json.RawMessage
		evidence   *Evidence
		complete   bool
		missing    []string
		orderID    string
		merchantNo string
	}{
		{
			name:     "平台单号命中",
			raw:      json.RawMessage(`{"order_id":" LPP3000456 "}`),
			evidence: &Evidence{UserInput: "订单 LPP3000456 查一下"},
			complete: true, orderID: "LPP3000456",
		},
		{
			name:     "只有商户单号",
			raw:      json.RawMessage(`{"merchant_order_no":"M-100"}`),
			evidence: &Evidence{UserInput: "商户单号 M-100"},
			complete: true, merchantNo: "M-100",
		},
		{
			name:     "两个都空",
			raw:      json.RawMessage(`{}`),
			evidence: &Evidence{UserInput: "查订单"},
			complete: false, missing: []string{"order_id", "merchant_order_no"},
		},
		{
			name:     "标识符原文不存在",
			raw:      json.RawMessage(`{"order_id":"FAKE-1"}`),
			evidence: &Evidence{UserInput: "订单 LPP3000456 查一下"},
			complete: false, missing: []string{"order_id"}, orderID: "FAKE-1",
		},
		{
			name:     "证据为 nil 跳过门闩",
			raw:      json.RawMessage(`{"order_id":"FAKE-1"}`),
			evidence: nil,
			complete: true, orderID: "FAKE-1",
		},
		{
			name:     "证据为空串执行门闩",
			raw:      json.RawMessage(`{"order_id":"FAKE-1"}`),
			evidence: &Evidence{},
			complete: false, missing: []string{"order_id"}, orderID: "FAKE-1",
		},
		{
			name:     "OCR 命中",
			raw:      json.RawMessage(`{"order_id":"lpp3000456"}`),
			evidence: &Evidence{OCR: []string{"截图：LPP3000456"}},
			complete: true, orderID: "lpp3000456",
		},
		{
			name:     "历史命中",
			raw:      json.RawMessage(`{"merchant_order_no":"M-200"}`),
			evidence: &Evidence{History: []EvidenceTurn{{Role: "user", Content: "上次是 M-200"}}},
			complete: true, merchantNo: "M-200",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			resolution, err := Resolve(json.RawMessage(c.raw), spec, contract, c.evidence)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if resolution.Complete != c.complete {
				t.Fatalf("Complete 不符: %+v", resolution)
			}
			if !sameStrings(resolution.Missing, c.missing) {
				t.Fatalf("Missing 不符: got %v want %v", resolution.Missing, c.missing)
			}
			in, _ := resolution.Input.(fillOrderInput)
			if in.OrderID != c.orderID || in.MerchantOrderNo != c.merchantNo {
				t.Fatalf("typed 入参不符: %+v", in)
			}
		})
	}
}

// TestResolveErrors 覆盖硬错误
func TestResolveErrors(t *testing.T) {
	t.Parallel()
	spec := fillOrderSpec()
	cases := []struct {
		name     string
		raw      json.RawMessage
		contract InputContract[fillOrderInput]
	}{
		{
			name:     "参数不是对象",
			raw:      json.RawMessage(`[]`),
			contract: fillOrderContract(nil),
		},
		{
			name:     "字段类型错误",
			raw:      json.RawMessage(`{"order_id":123}`),
			contract: fillOrderContract(nil),
		},
		{
			name: "Decode 错误",
			raw:  json.RawMessage(`{"order_id":"LPP-1"}`),
			contract: InputContract[fillOrderInput]{
				Decode: func(json.RawMessage, *Evidence) (fillOrderInput, error) {
					return fillOrderInput{}, errors.New("decode failed")
				},
			},
		},
		{
			name: "Normalize 错误",
			raw:  json.RawMessage(`{"order_id":"LPP-1"}`),
			contract: InputContract[fillOrderInput]{
				Normalize: func(fillOrderInput) (fillOrderInput, error) {
					return fillOrderInput{}, errors.New("normalize failed")
				},
			},
		},
		{
			name: "Validate 错误",
			raw:  json.RawMessage(`{"order_id":"LPP-1"}`),
			contract: InputContract[fillOrderInput]{
				Validate: func(fillOrderInput) ([]string, error) {
					return nil, errors.New("validate failed")
				},
			},
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve(c.raw, spec, c.contract, &Evidence{UserInput: "LPP-1"})
			requireCode(t, err, CodeInvalidArgument)
		})
	}
}

// TestResolveDefaults 覆盖无 Validate/Identify 时的默认行为
func TestResolveDefaults(t *testing.T) {
	t.Parallel()
	spec := Spec{
		Name:        "simple",
		Description: "默认校验",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"},"id":{"type":"string"}},"required":["city"]}`),
		Fill: FillSpec{
			Required:       []string{"city"},
			IdentifierKeys: []string{"id"},
		},
	}
	contract := InputContract[struct {
		City string `json:"city"`
		ID   string `json:"id"`
	}]{}
	if resolution, err := Resolve(json.RawMessage(`{"city":"北京"}`), spec, contract, &Evidence{UserInput: "北京"}); err != nil || !resolution.Complete {
		t.Fatalf("默认必填通过失败: %+v %v", resolution, err)
	}
	if resolution, err := Resolve(json.RawMessage(`{"city":""}`), spec, contract, &Evidence{UserInput: "北京"}); err != nil || resolution.Complete {
		t.Fatalf("空字符串应缺槽: %+v %v", resolution, err)
	}
	if resolution, err := Resolve(json.RawMessage(`{"city":"北京","id":"ABC"}`), spec, contract, &Evidence{UserInput: "abc"}); err != nil || !resolution.Complete {
		t.Fatalf("默认标识符应大小写不敏感: %+v %v", resolution, err)
	}
	if resolution, err := Resolve(json.RawMessage(`{"city":"北京","id":"ZZZ"}`), spec, contract, &Evidence{UserInput: "abc"}); err != nil || resolution.Complete {
		t.Fatalf("默认标识符不在证据中应缺槽: %+v %v", resolution, err)
	}
}

// TestResolvedInvoke 覆盖解析后执行、缺槽与 typed 执行
func TestResolvedInvoke(t *testing.T) {
	t.Parallel()
	var handlerCalls atomic.Int32
	current := newOrderTool(t, nil, &handlerCalls)
	registry := newRegistry(t, current)

	// 缺槽：handler 不执行
	_, err := registry.Invoke(context.Background(), Call{ID: "call-1", Name: "order_query", Arguments: json.RawMessage(`{}`)})
	toolErr := requireCode(t, err, CodeInputIncomplete)
	if !sameStrings(toolErr.Missing, []string{"order_id", "merchant_order_no"}) || handlerCalls.Load() != 0 {
		t.Fatalf("缺槽语义不符: %+v calls=%d", toolErr, handlerCalls.Load())
	}

	// 标识符臆造：handler 不执行
	_, err = registry.Invoke(context.Background(), Call{
		ID: "call-2", Name: "order_query", Arguments: json.RawMessage(`{"order_id":"FAKE-1"}`),
		Evidence: &Evidence{UserInput: "订单 LPP3000456"},
	})
	toolErr = requireCode(t, err, CodeInputIncomplete)
	if !sameStrings(toolErr.Missing, []string{"order_id"}) || handlerCalls.Load() != 0 {
		t.Fatalf("标识符门闩不符: %+v calls=%d", toolErr, handlerCalls.Load())
	}

	// 完整：handler 执行
	result, err := registry.Invoke(context.Background(), Call{
		ID: "call-3", Name: "order_query", Arguments: json.RawMessage(`{"order_id":"LPP3000456"}`),
		Evidence: &Evidence{UserInput: "订单 LPP3000456"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if handlerCalls.Load() != 1 || result.CallID != "call-3" || string(result.Content) != `{"order_id":"LPP3000456"}` {
		t.Fatalf("执行结果不符: %+v calls=%d", result, handlerCalls.Load())
	}

	// typed 执行：InvokeResolved 直接调用 handler
	resolved, ok := current.(ResolvedTool)
	if !ok {
		t.Fatal("NewResolved 应实现 ResolvedTool")
	}
	content, err := resolved.InvokeResolved(context.Background(), fillOrderInput{OrderID: "LPP-2"})
	if err != nil || string(content) != `{"order_id":"LPP-2"}` || handlerCalls.Load() != 2 {
		t.Fatalf("InvokeResolved 不符: %s %v calls=%d", content, err, handlerCalls.Load())
	}

	// InputResolver 可直接 Resolve
	resolver, ok := current.(InputResolver)
	if !ok {
		t.Fatal("NewResolved 应实现 InputResolver")
	}
	resolution, err := resolver.Resolve(json.RawMessage(`{"merchant_order_no":"M-1"}`), &Evidence{UserInput: "M-1"})
	if err != nil || !resolution.Complete {
		t.Fatalf("InputResolver.Resolve 不符: %+v %v", resolution, err)
	}
}

// TestEvidenceOnlyFill 覆盖纯证据映射
func TestEvidenceOnlyFill(t *testing.T) {
	t.Parallel()
	spec := fillOrderSpec()
	spec.Fill.EvidenceOnly = true
	var handlerCalls atomic.Int32
	current, err := NewResolved(spec, InputContract[fillOrderInput]{
		Decode: func(_ json.RawMessage, ev *Evidence) (fillOrderInput, error) {
			return fillOrderInput{MerchantOrderNo: strings.TrimSpace(ev.UserInput)}, nil
		},
		Validate: func(in fillOrderInput) ([]string, error) {
			if in.OrderID == "" && in.MerchantOrderNo == "" {
				return []string{"order_id", "merchant_order_no"}, nil
			}
			return nil, nil
		},
	}, func(_ context.Context, in fillOrderInput) (fillOrderOutput, error) {
		handlerCalls.Add(1)
		return fillOrderOutput{OrderID: in.MerchantOrderNo}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := newRegistry(t, current)
	filled, err := registry.Fill(context.Background(), Call{
		ID: "call-1", Name: "order_query",
		Evidence: &Evidence{UserInput: "M-100"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !filled.Complete || filled.Source != FillSourceEvidence || filled.Attempts != 0 {
		t.Fatalf("证据映射结果不符: %+v", filled)
	}
	if string(filled.Arguments) != `{"order_id":"","merchant_order_no":"M-100","not_created":false}` {
		t.Fatalf("证据映射 Arguments 不符: %s", filled.Arguments)
	}
	result, err := registry.InvokeFilled(context.Background(), filled)
	if err != nil || handlerCalls.Load() != 1 || string(result.Content) != `{"order_id":"M-100"}` {
		t.Fatalf("证据映射执行不符: %+v %v calls=%d", result, err, handlerCalls.Load())
	}

	// 证据不足：返回不完整，不执行
	emptyFilled, err := registry.Fill(context.Background(), Call{Name: "order_query", Evidence: &Evidence{}})
	if err != nil || emptyFilled.Complete {
		t.Fatalf("证据不足应返回不完整: %+v %v", emptyFilled, err)
	}
	if _, err := registry.InvokeFilled(context.Background(), emptyFilled); err == nil {
		t.Fatal("证据不足不应执行")
	} else {
		requireCode(t, err, CodeInputIncomplete)
	}

	// 证据映射错误：FillInvalid
	broken, err := NewResolved(spec, InputContract[fillOrderInput]{
		Decode: func(json.RawMessage, *Evidence) (fillOrderInput, error) {
			return fillOrderInput{}, errors.New("evidence decode failed")
		},
	}, func(context.Context, fillOrderInput) (fillOrderOutput, error) {
		return fillOrderOutput{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	brokenRegistry := newRegistry(t, broken)
	_, err = brokenRegistry.Fill(context.Background(), Call{Name: "order_query", Evidence: &Evidence{UserInput: "x"}})
	requireCode(t, err, CodeFillInvalid)

	// EvidenceOnly 没有自定义 Decode 时拒绝构造
	if _, err := NewResolved(spec, InputContract[fillOrderInput]{}, func(context.Context, fillOrderInput) (fillOrderOutput, error) {
		return fillOrderOutput{}, nil
	}); err == nil {
		t.Fatal("EvidenceOnly 无 Decode 应被拒")
	} else {
		requireCode(t, err, CodeInvalidArgument)
	}
}

// TestResolvedInvokeResolveError 覆盖解析错误带调用上下文
func TestResolvedInvokeResolveError(t *testing.T) {
	t.Parallel()
	registry := newRegistry(t, newOrderTool(t, nil, nil))
	_, err := registry.Invoke(context.Background(), Call{
		ID: "call-1", Name: "order_query", Arguments: json.RawMessage(`{"order_id":123}`),
	})
	toolErr := requireCode(t, err, CodeInvalidArgument)
	if toolErr.CallID != "call-1" || toolErr.Tool != "order_query" {
		t.Fatalf("错误上下文不符: %+v", toolErr)
	}
}

// TestInvokeFilledTypedPanic 覆盖 typed 执行 panic 兜底
func TestInvokeFilledTypedPanic(t *testing.T) {
	t.Parallel()
	current, err := NewResolved(fillOrderSpec(), fillOrderContract(nil),
		func(context.Context, fillOrderInput) (fillOrderOutput, error) {
			panic("typed panic detail")
		})
	if err != nil {
		t.Fatal(err)
	}
	registry := newRegistry(t, current)
	filled := &Filled{
		CallID: "call-1", Name: "order_query", Complete: true,
		Input: fillOrderInput{OrderID: "LPP-1"}, Arguments: json.RawMessage(`{"order_id":"LPP-1"}`),
	}
	_, err = registry.InvokeFilled(context.Background(), filled)
	toolErr := requireCode(t, err, CodePanic)
	if strings.Contains(toolErr.Error(), "typed panic detail") ||
		toolErr.Cause() == nil || !strings.Contains(toolErr.Cause().Error(), "typed panic detail") {
		t.Fatalf("panic 封装不符: %+v", toolErr)
	}
}

// TestInvokeFilledMissingTypedInput 覆盖 resolved 结果缺少 typed 入参
func TestInvokeFilledMissingTypedInput(t *testing.T) {
	t.Parallel()
	registry := newRegistry(t, newOrderTool(t, nil, nil))
	_, err := registry.InvokeFilled(context.Background(), &Filled{
		Name: "order_query", Complete: true, Arguments: json.RawMessage(`{"order_id":"LPP-1"}`),
	})
	requireCode(t, err, CodeInvalidArgument)
}

// TestNewResolvedValidation 覆盖构造期校验
func TestNewResolvedValidation(t *testing.T) {
	t.Parallel()
	if _, err := NewResolved[fillOrderInput, fillOrderOutput](fillOrderSpec(), fillOrderContract(nil), nil); err == nil {
		t.Fatal("nil 函数应被拒")
	} else {
		requireCode(t, err, CodeInvalidArgument)
	}
	if _, err := NewResolved(Spec{}, fillOrderContract(nil), func(context.Context, fillOrderInput) (fillOrderOutput, error) {
		return fillOrderOutput{}, nil
	}); err == nil {
		t.Fatal("非法定义应被拒")
	} else {
		requireCode(t, err, CodeInvalidArgument)
	}
	if _, err := Resolve[fillOrderInput](json.RawMessage(`{}`), Spec{}, InputContract[fillOrderInput]{}, nil); err == nil {
		t.Fatal("非法定义 Resolve 应被拒")
	} else {
		requireCode(t, err, CodeInvalidArgument)
	}
}

// sameStrings 按顺序比较两个字符串切片
func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
