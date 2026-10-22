package plugin

import (
	"context"
	"slices"
	"testing"
)

// basePlugin 提供 Plugin 接口的基础实现
type basePlugin struct {
	PluginState
}

// Init 空实现
func (p *basePlugin) Init(*Host) {}

// Close 空实现
func (p *basePlugin) Close(*Host) {}

// readyPlugin 只实现 ReadyHook
type readyPlugin struct {
	basePlugin
	calls *[]string
}

// OnReady 记录调用顺序
func (p *readyPlugin) OnReady(context.Context) error {
	*p.calls = append(*p.calls, p.Name())
	return nil
}

// drainPlugin 只实现 DrainHook
type drainPlugin struct {
	basePlugin
	calls *[]string
}

// OnDraining 记录调用顺序
func (p *drainPlugin) OnDraining(context.Context) error {
	*p.calls = append(*p.calls, p.Name())
	return nil
}

// bothPlugin 同时实现 ReadyHook 与 DrainHook
type bothPlugin struct {
	basePlugin
	readyCalls *[]string
	drainCalls *[]string
}

// OnReady 记录调用顺序
func (p *bothPlugin) OnReady(context.Context) error {
	*p.readyCalls = append(*p.readyCalls, p.Name())
	return nil
}

// OnDraining 记录调用顺序
func (p *bothPlugin) OnDraining(context.Context) error {
	*p.drainCalls = append(*p.drainCalls, p.Name())
	return nil
}

// newBase 创建带名称的基础插件
func newBase(name string) basePlugin {
	return basePlugin{PluginState: NewPluginState(name)}
}

// TestRuntimeHooks 验证 hook 收集顺序、逆序与 Started 过滤
func TestRuntimeHooks(t *testing.T) {
	var readyCalls []string
	var drainCalls []string

	a := &readyPlugin{basePlugin: newBase("a"), calls: &readyCalls}
	a.SetStarted()

	b := &bothPlugin{basePlugin: newBase("b"), readyCalls: &readyCalls, drainCalls: &drainCalls}
	b.SetStarted()

	c := &drainPlugin{basePlugin: newBase("c"), calls: &drainCalls}
	c.SetStarted()

	d := &readyPlugin{basePlugin: newBase("d"), calls: &readyCalls} // 未启动，应跳过

	e := &drainPlugin{basePlugin: newBase("e"), calls: &drainCalls}
	e.SetStarted()

	rt := &Runtime{started: []Plugin{a, b, c, d, e}}

	readyHooks := rt.ReadyHooks()
	if len(readyHooks) != 2 {
		t.Fatalf("ReadyHooks 数量 = %d, want 2", len(readyHooks))
	}
	for _, hook := range readyHooks {
		if err := hook(context.Background()); err != nil {
			t.Fatalf("Ready hook 执行失败: %v", err)
		}
	}
	if want := []string{"a", "b"}; !slices.Equal(readyCalls, want) {
		t.Fatalf("Ready 顺序 = %v, want %v", readyCalls, want)
	}

	drainHooks := rt.DrainHooks()
	if len(drainHooks) != 3 {
		t.Fatalf("DrainHooks 数量 = %d, want 3", len(drainHooks))
	}
	for _, hook := range drainHooks {
		if err := hook(context.Background()); err != nil {
			t.Fatalf("Drain hook 执行失败: %v", err)
		}
	}
	if want := []string{"e", "c", "b"}; !slices.Equal(drainCalls, want) {
		t.Fatalf("Drain 顺序 = %v, want %v", drainCalls, want)
	}
}

// TestRuntimeHooksNil 验证 nil Runtime 返回 nil
func TestRuntimeHooksNil(t *testing.T) {
	var rt *Runtime
	if rt.ReadyHooks() != nil {
		t.Fatal("nil Runtime 的 ReadyHooks 应为 nil")
	}
	if rt.DrainHooks() != nil {
		t.Fatal("nil Runtime 的 DrainHooks 应为 nil")
	}
}
