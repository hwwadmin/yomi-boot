package plugin

import (
	"context"
	"slices"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/config"
)

// basePlugin 提供 Plugin 接口的基础实现
type basePlugin struct {
	PluginState

	initOrder  *[]string
	closeOrder *[]string
}

// Init 记录初始化顺序
func (p *basePlugin) Init(*PluginSlot) {
	if p.initOrder != nil {
		*p.initOrder = append(*p.initOrder, p.Name())
	}
}

// Close 记录关闭顺序
func (p *basePlugin) Close() {
	if p.closeOrder != nil {
		*p.closeOrder = append(*p.closeOrder, p.Name())
	}
}

// newBase 创建带名称的基础插件
func newBase(name string) basePlugin {
	return basePlugin{PluginState: NewPluginState(name)}
}

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

// blockingPlugin 的 Close 会阻塞到 release 关闭后返回
type blockingPlugin struct {
	basePlugin
	release chan struct{}
}

// Close 等待测试释放，用于验证关闭超时
func (p *blockingPlugin) Close() {
	<-p.release
}

// providerPlugin 持有一个供后序插件读取的值
type providerPlugin struct {
	basePlugin
	value string
}

// Value 返回插件自包含的值
func (p *providerPlugin) Value() string {
	return p.value
}

// consumerPlugin 在 Init 中从插槽查找前序插件
type consumerPlugin struct {
	basePlugin
	got string
}

// Init 通过 PluginSlot 解析依赖
func (p *consumerPlugin) Init(slot *PluginSlot) {
	p.basePlugin.Init(slot)
	p.got = slot.MustPlugin("provider").(*providerPlugin).Value()
}

// newTestSlot 创建测试用插件插槽
func newTestSlot(t *testing.T, plugins ...Plugin) *PluginSlot {
	t.Helper()
	return NewPluginSlot(&config.Config{}, plugins)
}

// TestPluginSlotInitAndLookup 验证按序 Init、查找与 Started
func TestPluginSlotInitAndLookup(t *testing.T) {
	var order []string
	a := &readyPlugin{basePlugin: newBase("a")}
	a.initOrder = &order
	b := &readyPlugin{basePlugin: newBase("b")}
	b.initOrder = &order

	slot := newTestSlot(t, a, b)
	slot.Init()

	if want := []string{"a", "b"}; !slices.Equal(order, want) {
		t.Fatalf("Init 顺序 = %v, want %v", order, want)
	}
	if _, ok := slot.Plugin("a"); !ok {
		t.Fatal("Plugin(a) 应存在")
	}
	if _, ok := slot.Plugin("missing"); ok {
		t.Fatal("Plugin(missing) 不应存在")
	}
	if slot.MustPlugin("b") != b {
		t.Fatal("MustPlugin(b) 返回了错误插件")
	}
	if got := len(slot.Plugins()); got != 2 {
		t.Fatalf("Plugins 数量 = %d, want 2", got)
	}
	slot.Plugins()[0] = nil
	if slot.Plugins()[0] == nil {
		t.Fatal("Plugins 应返回副本")
	}
	if slot.Started("a") {
		t.Fatal("未 SetStarted 的插件不应被视为已启动")
	}
	a.SetStarted()
	if !slot.Started("a") {
		t.Fatal("SetStarted 后 Started(a) 应为 true")
	}
}

// TestPluginSlotRejectsInvalidPlugins 验证 nil、空名称与重名插件直接 panic
func TestPluginSlotRejectsInvalidPlugins(t *testing.T) {
	cases := []struct {
		name    string
		plugins []Plugin
	}{
		{name: "nil 插件", plugins: []Plugin{nil}},
		{name: "空名称", plugins: []Plugin{&readyPlugin{basePlugin: newBase("")}}},
		{name: "重名", plugins: []Plugin{
			&readyPlugin{basePlugin: newBase("dup")},
			&drainPlugin{basePlugin: newBase("dup")},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s 应 panic", tc.name)
				}
			}()
			newTestSlot(t, tc.plugins...)
		})
	}
}

// TestPluginSlotInitTwice 验证重复 Init 直接 panic
func TestPluginSlotInitTwice(t *testing.T) {
	slot := newTestSlot(t, &readyPlugin{basePlugin: newBase("a")})
	slot.Init()

	defer func() {
		if recover() == nil {
			t.Fatal("重复 Init 应 panic")
		}
	}()
	slot.Init()
}

// TestPluginSlotHooks 验证 hook 收集顺序、逆序与 Started 过滤
func TestPluginSlotHooks(t *testing.T) {
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

	slot := newTestSlot(t, a, b, c, d, e)

	readyHooks := slot.ReadyHooks()
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

	drainHooks := slot.DrainHooks()
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

// TestPluginSlotCloseReverse 验证逆序关闭且可重复调用
func TestPluginSlotCloseReverse(t *testing.T) {
	var order []string
	a := &readyPlugin{basePlugin: newBase("a")}
	a.closeOrder = &order
	b := &readyPlugin{basePlugin: newBase("b")}
	b.closeOrder = &order
	c := &readyPlugin{basePlugin: newBase("c")}
	c.closeOrder = &order

	slot := newTestSlot(t, a, b, c)
	slot.Close()
	slot.Close()

	if want := []string{"c", "b", "a"}; !slices.Equal(order, want) {
		t.Fatalf("Close 顺序 = %v, want %v", order, want)
	}
}

// TestPluginSlotCloseTimeoutContinues 验证单个插件关闭超时后仍继续关闭前序插件
func TestPluginSlotCloseTimeoutContinues(t *testing.T) {
	slow := &blockingPlugin{basePlugin: newBase("slow"), release: make(chan struct{})}

	var closed []string
	fast := &readyPlugin{basePlugin: newBase("fast")}
	fast.closeOrder = &closed

	// 逆序关闭：slow 先关闭并卡住，fast 必须在超时后仍被关闭
	slot := newTestSlot(t, fast, slow)
	slot.SetCloseTimeout(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		slot.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close 应在上限内返回，不应被卡住的插件拖住")
	}
	if want := []string{"fast"}; !slices.Equal(closed, want) {
		t.Fatalf("超时后应继续关闭前序插件，关闭顺序 = %v, want %v", closed, want)
	}
	close(slow.release)
}

// TestPluginSlotDependencyLookup 验证插件可在 Init 中查找前序插件组件
func TestPluginSlotDependencyLookup(t *testing.T) {
	provider := &providerPlugin{basePlugin: newBase("provider"), value: "ready"}
	consumer := &consumerPlugin{basePlugin: newBase("consumer")}

	slot := newTestSlot(t, provider, consumer)
	slot.Init()

	if consumer.got != "ready" {
		t.Fatalf("consumer 读取依赖 = %q, want %q", consumer.got, "ready")
	}
}

// TestPluginSlotNil 验证 nil PluginSlot 的只读方法安全
func TestPluginSlotNil(t *testing.T) {
	var slot *PluginSlot
	if _, ok := slot.Plugin("a"); ok {
		t.Fatal("nil slot 的 Plugin 应返回 false")
	}
	if slot.Started("a") {
		t.Fatal("nil slot 的 Started 应为 false")
	}
	if slot.ReadyHooks() != nil {
		t.Fatal("nil slot 的 ReadyHooks 应为 nil")
	}
	if slot.DrainHooks() != nil {
		t.Fatal("nil slot 的 DrainHooks 应为 nil")
	}
	if slot.Plugins() != nil {
		t.Fatal("nil slot 的 Plugins 应为 nil")
	}
	slot.Init()
	slot.Close()
}
