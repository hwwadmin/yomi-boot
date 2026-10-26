// 注册表、调用与批量执行测试。
//
// @author zwei

package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// noArgsSpec 返回无参数工具定义
func noArgsSpec(name string) Spec {
	return Spec{
		Name:        name,
		Description: "无参数工具",
		Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

// newTool 构造函数工具并断言成功
func newTool(t *testing.T, spec Spec, fn Func) Tool {
	t.Helper()
	current, err := NewFunc(spec, fn)
	if err != nil {
		t.Fatalf("构造工具失败: %v", err)
	}
	return current
}

// newRegistry 构造注册表并断言成功
func newRegistry(t *testing.T, tools ...Tool) *Registry {
	t.Helper()
	registry, err := New(Config{Tools: tools})
	if err != nil {
		t.Fatalf("构造注册表失败: %v", err)
	}
	return registry
}

// stubTool 是用于注册表校验测试的最小实现
type stubTool struct {
	spec   Spec
	invoke func(context.Context, Call) (json.RawMessage, error)
}

// Spec 实现 Tool
func (s stubTool) Spec() Spec {
	return s.spec
}

// Invoke 实现 Tool
func (s stubTool) Invoke(ctx context.Context, call Call) (json.RawMessage, error) {
	if s.invoke == nil {
		return json.RawMessage(`{}`), nil
	}
	return s.invoke(ctx, call)
}

// panicSpecTool 的 Spec 会 panic，用于构造期兜底测试
type panicSpecTool struct{}

// Spec 实现 Tool
func (panicSpecTool) Spec() Spec {
	panic("spec panicked")
}

// Invoke 实现 Tool
func (panicSpecTool) Invoke(context.Context, Call) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// TestNewRegistry 覆盖构造校验、定义快照与查找
func TestNewRegistry(t *testing.T) {
	t.Parallel()
	alpha := newTool(t, validSpec("alpha"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	beta := newTool(t, validSpec("beta"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	registry := newRegistry(t, alpha, beta)

	specs := registry.Specs()
	if len(specs) != 2 || specs[0].Name != "alpha" || specs[1].Name != "beta" {
		t.Fatalf("Specs 顺序不符: %+v", specs)
	}
	specs[0].Metadata["risk"] = "write"
	specs[0].Parameters[0] = '['
	again := registry.Specs()
	if again[0].Metadata["risk"] != "read" || again[0].Parameters[0] != '{' {
		t.Fatal("Specs 应为深拷贝")
	}
	if _, ok := registry.Lookup(" alpha "); !ok {
		t.Fatal("Lookup 应 trim 后命中")
	}
	if _, ok := registry.Lookup("ALPHA"); ok {
		t.Fatal("Lookup 应大小写敏感")
	}

	upper := newTool(t, validSpec("ALPHA"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	cases := []struct {
		name string
		cfg  Config
	}{
		{"重复名", Config{Tools: []Tool{alpha, alpha}}},
		{"仅大小写重复", Config{Tools: []Tool{alpha, upper}}},
		{"nil 工具", Config{Tools: []Tool{nil}}},
		{"非法定义", Config{Tools: []Tool{stubTool{spec: Spec{}}}}},
		{"Spec panic", Config{Tools: []Tool{panicSpecTool{}}}},
		{"参数上限为负", Config{MaxArgumentsBytes: -1}},
		{"内容上限为负", Config{MaxContentBytes: -1}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(c.cfg)
			requireCode(t, err, CodeInvalidArgument)
		})
	}

	empty, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if empty.maxArgumentsBytes != DefaultMaxArgumentsBytes || empty.maxContentBytes != DefaultMaxContentBytes {
		t.Fatal("默认上限未回退")
	}
	if len(empty.Specs()) != 0 {
		t.Fatal("空注册表应返回空 Specs")
	}
	if _, err := empty.Invoke(context.Background(), Call{Name: "missing"}); err == nil {
		t.Fatal("空注册表调用应返回未注册")
	} else {
		requireCode(t, err, CodeToolNotFound)
	}
}

// TestCustomTool 覆盖自定义 Tool 实现接入注册表
func TestCustomTool(t *testing.T) {
	t.Parallel()
	custom := stubTool{
		spec: validSpec("custom"),
		invoke: func(_ context.Context, call Call) (json.RawMessage, error) {
			if call.Name != "custom" || call.ID != "call-1" {
				t.Fatalf("调用入参不符: %+v", call)
			}
			return json.RawMessage(`{"custom":true}`), nil
		},
	}
	registry := newRegistry(t, custom)
	result, err := registry.Invoke(context.Background(), Call{
		ID:        "call-1",
		Name:      "custom",
		Arguments: json.RawMessage(`{"city":"北京"}`),
	})
	if err != nil || string(result.Content) != `{"custom":true}` {
		t.Fatalf("自定义工具执行不符: %+v %v", result, err)
	}
}

// TestMustNewPanics 覆盖非法配置 Fail Fast
func TestMustNewPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("非法配置应 panic")
		}
	}()
	MustNew(Config{Tools: []Tool{stubTool{spec: Spec{}}}})
}

// TestNilRegistry 覆盖 nil 与零值注册表的安全行为
func TestNilRegistry(t *testing.T) {
	t.Parallel()
	var registry *Registry
	if registry.Specs() != nil {
		t.Fatal("nil 注册表 Specs 应为 nil")
	}
	if _, ok := registry.Lookup("x"); ok {
		t.Fatal("nil 注册表 Lookup 应未命中")
	}
	_, err := registry.Invoke(context.Background(), Call{Name: "x"})
	requireCode(t, err, CodeInvalidArgument)
	outcomes := registry.InvokeAll(context.Background(), []Call{{Name: "x"}}, 1)
	if len(outcomes) != 1 || outcomes[0].Err == nil {
		t.Fatalf("nil 注册表批量调用应返回错误: %+v", outcomes)
	}

	var zero Registry
	if len(zero.Specs()) != 0 {
		t.Fatal("零值注册表应返回空 Specs")
	}
	_, err = zero.Invoke(context.Background(), Call{Name: "x"})
	requireCode(t, err, CodeToolNotFound)
}

// TestInvoke 覆盖成功调用、参数归一与基本错误
func TestInvoke(t *testing.T) {
	t.Parallel()
	registry := newRegistry(t, newTool(t, validSpec("get_weather"), func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if string(args) != `{"city":"北京"}` {
			t.Fatalf("参数不符: %s", args)
		}
		return json.RawMessage(`{"weather":"晴"}`), nil
	}))

	result, err := registry.Invoke(context.Background(), Call{
		ID:        "call-1",
		Name:      " get_weather ",
		Arguments: json.RawMessage(`{"city":"北京"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CallID != "call-1" || result.Name != "get_weather" || string(result.Content) != `{"weather":"晴"}` {
		t.Fatalf("结果不符: %+v", result)
	}

	var received []string
	ping := newTool(t, noArgsSpec("ping"), func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		received = append(received, string(args))
		return json.RawMessage(`{}`), nil
	})
	pingRegistry := newRegistry(t, ping)
	for _, args := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`  `)} {
		if _, err := pingRegistry.Invoke(context.Background(), Call{Name: "ping", Arguments: args}); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range received {
		if args != `{}` {
			t.Fatalf("空参数应归一为 {}: %s", args)
		}
	}

	for _, call := range []Call{
		{ID: "c", Name: "get_weather", Arguments: json.RawMessage(`{}`)},
		{ID: "c", Name: "get_weather", Arguments: json.RawMessage(`[]`)},
		{ID: "c", Name: "get_weather", Arguments: json.RawMessage(`{"city":`)},
		{ID: "c", Name: "  "},
		{ID: "c", Name: "missing", Arguments: json.RawMessage(`{}`)},
	} {
		_, err := registry.Invoke(context.Background(), call)
		if call.Name == "missing" {
			requireCode(t, err, CodeToolNotFound)
			continue
		}
		requireCode(t, err, CodeInvalidArgument)
	}
}

// TestInvokeLimits 覆盖参数与结果上限
func TestInvokeLimits(t *testing.T) {
	t.Parallel()
	argsTool := newTool(t, validSpec("get_weather"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	argsRegistry, err := New(Config{Tools: []Tool{argsTool}, MaxArgumentsBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	_, err = argsRegistry.Invoke(context.Background(), Call{Name: "get_weather", Arguments: json.RawMessage(`{"city":"北京"}`)})
	toolErr := requireCode(t, err, CodeInvalidArgument)
	if toolErr.Message != defaultArgumentsTooLargeMessage {
		t.Fatalf("参数超限文案不符: %s", toolErr.Message)
	}

	contentTool := newTool(t, noArgsSpec("big"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"weather":"晴"}`), nil
	})
	contentRegistry, err := New(Config{Tools: []Tool{contentTool}, MaxContentBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	_, err = contentRegistry.Invoke(context.Background(), Call{Name: "big", Arguments: json.RawMessage(`{}`)})
	toolErr = requireCode(t, err, CodeBadResponse)
	if toolErr.Message != defaultContentTooLargeMessage {
		t.Fatalf("内容超限文案不符: %s", toolErr.Message)
	}
}

// TestInvokeContent 覆盖结果归一、非法 JSON 与防御性拷贝
func TestInvokeContent(t *testing.T) {
	t.Parallel()
	var shared = json.RawMessage(`{"ok":true}`)
	current := newTool(t, noArgsSpec("copy"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return shared, nil
	})
	registry := newRegistry(t, current)
	result, err := registry.Invoke(context.Background(), Call{Name: "copy", Arguments: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	shared[0] = '['
	if string(result.Content) != `{"ok":true}` {
		t.Fatal("结果未做防御性拷贝")
	}

	empty := newTool(t, noArgsSpec("empty"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	})
	emptyRegistry := newRegistry(t, empty)
	result, err = emptyRegistry.Invoke(context.Background(), Call{Name: "empty", Arguments: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Content) != "null" {
		t.Fatalf("空结果应归一为 null: %s", result.Content)
	}

	invalid := newTool(t, noArgsSpec("invalid"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`not-json`), nil
	})
	invalidRegistry := newRegistry(t, invalid)
	_, err = invalidRegistry.Invoke(context.Background(), Call{Name: "invalid", Arguments: json.RawMessage(`{}`)})
	requireCode(t, err, CodeBadResponse)

	original := json.RawMessage(`{"city":"北京"}`)
	mutating := newTool(t, validSpec("mutate"), func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		args[0] = '['
		return json.RawMessage(`{}`), nil
	})
	mutatingRegistry := newRegistry(t, mutating)
	if _, err := mutatingRegistry.Invoke(context.Background(), Call{Name: "mutate", Arguments: original}); err != nil {
		t.Fatal(err)
	}
	if string(original) != `{"city":"北京"}` {
		t.Fatal("参数未做防御性拷贝")
	}
}

// rawBizError 是底层业务错误，不应直接暴露
type rawBizError struct {
	text string
}

// Error 实现 error
func (e *rawBizError) Error() string {
	return e.text
}

// safeBizError 提供安全文案
type safeBizError struct {
	raw  string
	safe string
}

// Error 实现 error
func (e *safeBizError) Error() string {
	return e.raw
}

// SafeMessage 实现 SafeError
func (e *safeBizError) SafeMessage() string {
	return e.safe
}

// TestInvokeEncapsulation 覆盖底层错误不暴露、Cause 可取与安全文案
func TestInvokeEncapsulation(t *testing.T) {
	t.Parallel()
	raw := &rawBizError{text: "raw secret detail"}
	registry := newRegistry(t, newTool(t, validSpec("get_weather"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, raw
	}))
	_, err := registry.Invoke(context.Background(), Call{ID: "call-1", Name: "get_weather", Arguments: json.RawMessage(`{"city":"北京"}`)})
	toolErr := requireCode(t, err, CodeExecution)
	if toolErr.Tool != "get_weather" || toolErr.CallID != "call-1" {
		t.Fatalf("工具名或调用标识不符: %+v", toolErr)
	}
	if strings.Contains(toolErr.Error(), raw.text) {
		t.Fatal("Error() 不应包含底层错误")
	}
	var target *rawBizError
	if errors.As(err, &target) {
		t.Fatal("不应能 As 到底层业务错误")
	}
	if !strings.Contains(toolErr.Cause().Error(), raw.text) {
		t.Fatal("Cause 应保留底层错误")
	}
	serialized, marshalErr := json.Marshal(toolErr)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(serialized), raw.text) {
		t.Fatal("JSON 不应包含底层错误")
	}
	if !errors.Is(err, ErrExecution) || errors.Is(err, ErrInvalidArgument) {
		t.Fatal("errors.Is 分类不符")
	}

	safe := &safeBizError{raw: "raw secret detail", safe: "城市不存在"}
	safeRegistry := newRegistry(t, newTool(t, validSpec("get_weather"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, safe
	}))
	_, err = safeRegistry.Invoke(context.Background(), Call{Name: "get_weather", Arguments: json.RawMessage(`{"city":"北京"}`)})
	toolErr = requireCode(t, err, CodeExecution)
	if toolErr.Message != "城市不存在" || strings.Contains(toolErr.Error(), safe.raw) {
		t.Fatalf("SafeError 文案不符: %+v", toolErr)
	}

	nested := newError(CodeInvalidArgument, "城市不合法", "inner", "inner-call", errors.New("inner raw"))
	nestedRegistry := newRegistry(t, newTool(t, validSpec("get_weather"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, nested
	}))
	_, err = nestedRegistry.Invoke(context.Background(), Call{Name: "get_weather", Arguments: json.RawMessage(`{"city":"北京"}`)})
	toolErr = requireCode(t, err, CodeInvalidArgument)
	if toolErr.Message != "城市不合法" || toolErr.Tool != "get_weather" || toolErr.CallID != "" {
		t.Fatalf("嵌套错误应保留错误码与安全文案: %+v", toolErr)
	}
	if toolErr.Cause() == nil || toolErr.Cause().Error() != "inner raw" {
		t.Fatalf("嵌套错误原因应摊平: %v", toolErr.Cause())
	}

	// 工具实现返回未注册哨兵或未知错误码时，按执行失败处理，避免伪造 Registry 语义
	for _, nestedErr := range []*Error{
		newError(CodeToolNotFound, "工具未注册", "inner", "", errors.New("inner raw")),
		newError("AI_TOOL_UNKNOWN", "未知错误", "inner", "", errors.New("inner raw")),
	} {
		nestedErr := nestedErr
		nestedRegistry := newRegistry(t, newTool(t, validSpec("get_weather"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, nestedErr
		}))
		_, err = nestedRegistry.Invoke(context.Background(), Call{Name: "get_weather", Arguments: json.RawMessage(`{"city":"北京"}`)})
		toolErr = requireCode(t, err, CodeExecution)
		if toolErr.Message != defaultExecutionMessage {
			t.Fatalf("未知错误码应回退安全文案: %+v", toolErr)
		}
	}
}

// TestInvokeClassifiedErrors 覆盖哨兵、上下文与 panic 分类
func TestInvokeClassifiedErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		err    error
		code   string
		cause  error
		isExec bool
	}{
		{"非法参数", ErrInvalidArgument, CodeInvalidArgument, ErrInvalidArgument, false},
		{"非法输出", ErrBadResponse, CodeBadResponse, ErrBadResponse, true},
		{"超时", context.DeadlineExceeded, CodeTimeout, context.DeadlineExceeded, true},
		{"取消", context.Canceled, CodeCanceled, context.Canceled, true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			registry := newRegistry(t, newTool(t, validSpec("get_weather"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return nil, c.err
			}))
			_, err := registry.Invoke(context.Background(), Call{Name: "get_weather", Arguments: json.RawMessage(`{"city":"北京"}`)})
			toolErr := requireCode(t, err, c.code)
			if !errors.Is(toolErr.Cause(), c.cause) {
				t.Fatalf("原因不符: %v", toolErr.Cause())
			}
			if errors.Is(err, ErrExecution) != c.isExec {
				t.Fatalf("ErrExecution 分类不符: %v", err)
			}
		})
	}

	registry := newRegistry(t, newTool(t, noArgsSpec("panic"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		panic("panic secret detail")
	}))
	_, err := registry.Invoke(context.Background(), Call{Name: "panic", Arguments: json.RawMessage(`{}`)})
	toolErr := requireCode(t, err, CodePanic)
	if strings.Contains(toolErr.Error(), "panic secret detail") {
		t.Fatal("Error() 不应包含 panic 值")
	}
	if toolErr.Cause() == nil || !strings.Contains(toolErr.Cause().Error(), "panic secret detail") {
		t.Fatal("Cause 应包含 panic 值")
	}
	if !errors.Is(err, ErrPanic) || !errors.Is(err, ErrExecution) {
		t.Fatal("panic 分类不符")
	}
}

// TestInvokeCanceledContext 覆盖调用前已取消
func TestInvokeCanceledContext(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	registry := newRegistry(t, newTool(t, noArgsSpec("ping"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		called.Add(1)
		return json.RawMessage(`{}`), nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := registry.Invoke(ctx, Call{Name: "ping", Arguments: json.RawMessage(`{}`)})
	requireCode(t, err, CodeCanceled)
	if called.Load() != 0 {
		t.Fatal("已取消的 ctx 不应执行工具")
	}
}

// TestInvokeAll 覆盖顺序、单点失败与不 fail-fast
func TestInvokeAll(t *testing.T) {
	t.Parallel()
	ok := newTool(t, noArgsSpec("ok"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	})
	fail := newTool(t, noArgsSpec("fail"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("boom")
	})
	registry := newRegistry(t, ok, fail)
	calls := []Call{
		{ID: "1", Name: "ok", Arguments: json.RawMessage(`{}`)},
		{ID: "2", Name: "missing", Arguments: json.RawMessage(`{}`)},
		{ID: "3", Name: "fail", Arguments: json.RawMessage(`{}`)},
		{ID: "4", Name: "ok", Arguments: json.RawMessage(`{}`)},
	}
	outcomes := registry.InvokeAll(context.Background(), calls, 0)
	if len(outcomes) != len(calls) {
		t.Fatalf("结局数量不符: %d", len(outcomes))
	}
	for i, outcome := range outcomes {
		if outcome.Call.ID != calls[i].ID {
			t.Fatalf("结局顺序不符: index %d", i)
		}
	}
	if outcomes[0].Err != nil || outcomes[0].Result == nil || outcomes[0].Result.CallID != "1" {
		t.Fatalf("成功结局不符: %+v", outcomes[0])
	}
	requireCode(t, outcomes[1].Err, CodeToolNotFound)
	requireCode(t, outcomes[2].Err, CodeExecution)
	if outcomes[3].Err != nil {
		t.Fatalf("单点失败不应影响后续调用: %+v", outcomes[3])
	}
	if outcomes := registry.InvokeAll(context.Background(), nil, 4); outcomes != nil {
		t.Fatal("空调用应返回 nil")
	}
}

// TestInvokeAllConcurrency 覆盖默认顺序执行与显式并发上限
func TestInvokeAllConcurrency(t *testing.T) {
	t.Parallel()
	measure := func(t *testing.T, concurrency int, want int) {
		t.Helper()
		var inFlight, maxInFlight atomic.Int32
		started := make(chan struct{}, 8)
		release := make(chan struct{})
		var once sync.Once
		releaseAll := func() { once.Do(func() { close(release) }) }
		current := newTool(t, noArgsSpec("slow"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
			n := inFlight.Add(1)
			for {
				old := maxInFlight.Load()
				if n <= old || maxInFlight.CompareAndSwap(old, n) {
					break
				}
			}
			started <- struct{}{}
			<-release
			inFlight.Add(-1)
			return json.RawMessage(`{}`), nil
		})
		registry := newRegistry(t, current)
		if want == 1 {
			releaseAll()
		} else {
			go func() {
				timer := time.NewTimer(2 * time.Second)
				defer timer.Stop()
				for i := 0; i < want; i++ {
					select {
					case <-started:
					case <-timer.C:
						releaseAll()
						return
					}
				}
				releaseAll()
			}()
		}
		outcomes := registry.InvokeAll(context.Background(), []Call{
			{Name: "slow"}, {Name: "slow"}, {Name: "slow"}, {Name: "slow"},
		}, concurrency)
		releaseAll()
		for _, outcome := range outcomes {
			if outcome.Err != nil {
				t.Fatalf("批量调用失败: %v", outcome.Err)
			}
		}
		if got := int(maxInFlight.Load()); got != want {
			t.Fatalf("并发度不符: got %d want %d", got, want)
		}
	}
	t.Run("默认顺序", func(t *testing.T) { measure(t, 0, 1) })
	t.Run("显式并发", func(t *testing.T) { measure(t, 2, 2) })
}

// TestInvokeAllCanceled 覆盖批量调用前取消与批中取消
func TestInvokeAllCanceled(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	registry := newRegistry(t, newTool(t, noArgsSpec("ping"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		called.Add(1)
		return json.RawMessage(`{}`), nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcomes := registry.InvokeAll(ctx, []Call{{Name: "ping"}, {Name: "ping"}}, 2)
	if called.Load() != 0 {
		t.Fatal("已取消的 ctx 不应执行工具")
	}
	for _, outcome := range outcomes {
		requireCode(t, outcome.Err, CodeCanceled)
	}

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	var midCalled atomic.Int32
	mid := newTool(t, noArgsSpec("mid"), func(context.Context, json.RawMessage) (json.RawMessage, error) {
		if midCalled.Add(1) == 1 {
			cancel()
		}
		return json.RawMessage(`{}`), nil
	})
	midRegistry := newRegistry(t, mid)
	outcomes = midRegistry.InvokeAll(ctx, []Call{{Name: "mid"}, {Name: "mid"}, {Name: "mid"}}, 1)
	if midCalled.Load() != 1 {
		t.Fatalf("取消后不应再启动新调用: %d", midCalled.Load())
	}
	if outcomes[0].Err != nil {
		t.Fatalf("首个调用应成功: %+v", outcomes[0])
	}
	requireCode(t, outcomes[1].Err, CodeCanceled)
	requireCode(t, outcomes[2].Err, CodeCanceled)
}

// TestErrorIs 覆盖错误码与哨兵匹配
func TestErrorIs(t *testing.T) {
	t.Parallel()
	if !errors.Is(newError(CodeExecution, "", "x", "", nil), ErrExecution) {
		t.Fatal("执行错误应匹配 ErrExecution")
	}
	if !errors.Is(newError(CodeTimeout, "", "x", "", nil), ErrExecution) {
		t.Fatal("超时应匹配 ErrExecution")
	}
	if !errors.Is(newError(CodeInvalidArgument, "", "x", "", nil), ErrInvalidArgument) {
		t.Fatal("参数错误应匹配 ErrInvalidArgument")
	}
	if !errors.Is(newError(CodeToolNotFound, "", "x", "", nil), ErrToolNotFound) {
		t.Fatal("未注册应匹配 ErrToolNotFound")
	}
	if !errors.Is(newError(CodeBadResponse, "", "x", "", nil), ErrBadResponse) {
		t.Fatal("非法输出应匹配 ErrBadResponse")
	}
	if !errors.Is(newError(CodePanic, "", "x", "", nil), ErrPanic) {
		t.Fatal("panic 应匹配 ErrPanic")
	}
	if !errors.Is(newError(CodeCanceled, "", "x", "", nil), context.Canceled) {
		t.Fatal("取消错误应匹配 context.Canceled")
	}
	if !errors.Is(newError(CodeTimeout, "", "x", "", nil), context.DeadlineExceeded) {
		t.Fatal("超时错误应匹配 context.DeadlineExceeded")
	}
	if errors.Is(newError(CodeExecution, "", "x", "", nil), ErrInvalidArgument) {
		t.Fatal("执行错误不应匹配 ErrInvalidArgument")
	}
	if !errors.Is(newError(CodeExecution, "", "x", "", nil), &Error{Code: CodeExecution}) {
		t.Fatal("同 Code 应匹配")
	}
	var nilErr *Error
	if nilErr.Error() != "" || nilErr.Cause() != nil || nilErr.SafeMessage() != "" {
		t.Fatal("nil Error 应安全")
	}
}
