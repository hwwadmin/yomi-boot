package plugin

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// basePlugin 提供 Plugin 接口的测试实现。
type basePlugin struct {
	Base

	initOrder  *[]string
	closeOrder *[]string
	initErr    error
	closeErr   error
	initPanic  bool
	closePanic bool
	blockInit  chan struct{}
	blockClose chan struct{}
}

// Init 按配置记录初始化顺序或返回错误。
func (p *basePlugin) Init(ctx context.Context, _ *PluginSlot) error {
	if p.blockInit != nil {
		select {
		case <-p.blockInit:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if p.initPanic {
		panic("init panic")
	}
	if p.initErr != nil {
		return p.initErr
	}
	if p.initOrder != nil {
		*p.initOrder = append(*p.initOrder, p.Name())
	}
	return nil
}

// Close 按配置记录关闭顺序或返回错误。
func (p *basePlugin) Close(ctx context.Context) error {
	if p.blockClose != nil {
		select {
		case <-p.blockClose:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if p.closePanic {
		panic("close panic")
	}
	if p.closeOrder != nil {
		*p.closeOrder = append(*p.closeOrder, p.Name())
	}
	return p.closeErr
}

// newBase 创建带名称的基础插件。
func newBase(name string) basePlugin {
	return basePlugin{Base: NewBase(name)}
}

// readyPlugin 只实现 ReadyHook。
type readyPlugin struct {
	basePlugin
	calls *[]string
}

// OnReady 记录调用顺序。
func (p *readyPlugin) OnReady(context.Context) error {
	*p.calls = append(*p.calls, p.Name())
	return nil
}

// drainPlugin 只实现 DrainHook。
type drainPlugin struct {
	basePlugin
	calls *[]string
}

// OnDraining 记录调用顺序。
func (p *drainPlugin) OnDraining(context.Context) error {
	*p.calls = append(*p.calls, p.Name())
	return nil
}

// bothPlugin 同时实现 ReadyHook 与 DrainHook。
type bothPlugin struct {
	basePlugin
	readyCalls *[]string
	drainCalls *[]string
}

// OnReady 记录调用顺序。
func (p *bothPlugin) OnReady(context.Context) error {
	*p.readyCalls = append(*p.readyCalls, p.Name())
	return nil
}

// OnDraining 记录调用顺序。
func (p *bothPlugin) OnDraining(context.Context) error {
	*p.drainCalls = append(*p.drainCalls, p.Name())
	return nil
}

// providerPlugin 持有一个供后序插件读取的值。
type providerPlugin struct {
	basePlugin
	value string
}

// Value 返回插件自包含的值。
func (p *providerPlugin) Value() string {
	return p.value
}

// consumerPlugin 在 Init 中从插槽查找前序插件。
type consumerPlugin struct {
	basePlugin
	got string
}

// Init 通过 PluginSlot 解析依赖。
func (p *consumerPlugin) Init(ctx context.Context, slot *PluginSlot) error {
	if err := p.basePlugin.Init(ctx, slot); err != nil {
		return err
	}
	p.got = slot.MustPlugin("provider").(*providerPlugin).Value()
	return nil
}

// newTestSlot 创建测试用插件插槽。
func newTestSlot(t *testing.T, plugins ...Plugin) *PluginSlot {
	t.Helper()
	return NewPluginSlot(&config.Config{}, plugins)
}

// TestPluginSlotInitAndLookup 验证按序 Init、查找与 Started。
func TestPluginSlotInitAndLookup(t *testing.T) {
	var order []string
	a := &readyPlugin{basePlugin: newBase("a")}
	a.initOrder = &order
	b := &readyPlugin{basePlugin: newBase("b")}
	b.initOrder = &order

	slot := newTestSlot(t, a, b)
	if slot.Started("a") {
		t.Fatal("Init 前不应被视为已初始化")
	}
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}

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
	if !slot.Started("a") || !slot.Started("b") {
		t.Fatal("Init 成功后插件应为已初始化")
	}
}

// TestPluginSlotRejectsInvalidPlugins 验证 nil、空名称与重名插件直接 panic。
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

// TestPluginSlotInitTwice 验证重复 Init 返回错误。
func TestPluginSlotInitTwice(t *testing.T) {
	slot := newTestSlot(t, &readyPlugin{basePlugin: newBase("a")})
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("首次 Init 失败: %v", err)
	}
	if err := slot.Init(context.Background()); err == nil || !strings.Contains(err.Error(), "不能重复 Init") {
		t.Fatalf("重复 Init 应返回错误，实际 %v", err)
	}
}

// TestPluginSlotInitAfterClose 验证 Close 后不能再 Init。
func TestPluginSlotInitAfterClose(t *testing.T) {
	slot := newTestSlot(t, &readyPlugin{basePlugin: newBase("a")})
	if err := slot.Close(context.Background()); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}
	if err := slot.Init(context.Background()); err == nil || !strings.Contains(err.Error(), "已关闭") {
		t.Fatalf("Close 后 Init 应返回错误，实际 %v", err)
	}
}

// TestPluginSlotHooks 验证 hook 收集顺序、逆序与跳过插件过滤。
func TestPluginSlotHooks(t *testing.T) {
	var readyCalls []string
	var drainCalls []string

	a := &readyPlugin{basePlugin: newBase("a"), calls: &readyCalls}
	b := &bothPlugin{basePlugin: newBase("b"), readyCalls: &readyCalls, drainCalls: &drainCalls}
	c := &drainPlugin{basePlugin: newBase("c"), calls: &drainCalls}
	d := &readyPlugin{basePlugin: newBase("d"), calls: &readyCalls}
	d.initErr = ErrDisabled
	e := &drainPlugin{basePlugin: newBase("e"), calls: &drainCalls}

	slot := newTestSlot(t, a, b, c, d, e)
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	if slot.Started("d") {
		t.Fatal("ErrDisabled 插件不应被视为已初始化")
	}

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

// TestPluginSlotCloseReverse 验证逆序关闭且可重复调用。
func TestPluginSlotCloseReverse(t *testing.T) {
	var order []string
	a := &readyPlugin{basePlugin: newBase("a")}
	a.closeOrder = &order
	b := &readyPlugin{basePlugin: newBase("b")}
	b.closeOrder = &order
	c := &readyPlugin{basePlugin: newBase("c")}
	c.closeOrder = &order

	slot := newTestSlot(t, a, b, c)
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	if err := slot.Close(context.Background()); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}
	if err := slot.Close(context.Background()); err != nil {
		t.Fatalf("重复 Close 失败: %v", err)
	}

	if want := []string{"c", "b", "a"}; !slices.Equal(order, want) {
		t.Fatalf("Close 顺序 = %v, want %v", order, want)
	}
}

// TestPluginSlotCloseTimeoutContinues 验证单个插件关闭超时后仍继续关闭前序插件。
func TestPluginSlotCloseTimeoutContinues(t *testing.T) {
	slow := &basePlugin{Base: NewBase("slow"), blockClose: make(chan struct{})}

	var closed []string
	fast := &readyPlugin{basePlugin: newBase("fast")}
	fast.closeOrder = &closed

	slot := newTestSlot(t, fast, slow)
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := slot.Close(ctx)
	if err == nil {
		t.Fatal("慢插件超时后 Close 应返回错误")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("Close 应在上限内返回")
	}
	if want := []string{"fast"}; !slices.Equal(closed, want) {
		t.Fatalf("超时后应继续关闭前序插件，关闭顺序 = %v, want %v", closed, want)
	}
	close(slow.blockClose)
}

// TestPluginSlotInitFailureRollsBack 验证 Init 失败时逆序回滚已初始化插件。
func TestPluginSlotInitFailureRollsBack(t *testing.T) {
	var closed []string
	a := &readyPlugin{basePlugin: newBase("a")}
	a.closeOrder = &closed
	fail := &basePlugin{Base: NewBase("fail"), initErr: errors.New("boom")}
	c := &readyPlugin{basePlugin: newBase("c")}

	slot := newTestSlot(t, a, fail, c)
	err := slot.Init(context.Background())
	if err == nil || !strings.Contains(err.Error(), "fail") {
		t.Fatalf("Init 应返回失败插件错误，实际 %v", err)
	}
	if want := []string{"a"}; !slices.Equal(closed, want) {
		t.Fatalf("回滚顺序 = %v, want %v", closed, want)
	}
	if slot.Started("a") || slot.Started("fail") || slot.Started("c") {
		t.Fatal("Init 失败后插件都不应处于已初始化状态")
	}
	if err := slot.Close(context.Background()); err != nil {
		t.Fatalf("Init 失败后 Close 应为空操作: %v", err)
	}
}

// TestPluginSlotInitPanicRollsBack 验证 Init panic 转为错误并回滚。
func TestPluginSlotInitPanicRollsBack(t *testing.T) {
	var closed []string
	a := &readyPlugin{basePlugin: newBase("a")}
	a.closeOrder = &closed
	fail := &basePlugin{Base: NewBase("fail"), initPanic: true}

	slot := newTestSlot(t, a, fail)
	err := slot.Init(context.Background())
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("Init panic 应转为错误，实际 %v", err)
	}
	if want := []string{"a"}; !slices.Equal(closed, want) {
		t.Fatalf("回滚顺序 = %v, want %v", closed, want)
	}
}

// TestPluginSlotInitCanceled 验证 ctx 已取消时 Init 返回取消错误。
func TestPluginSlotInitCanceled(t *testing.T) {
	var closed []string
	a := &readyPlugin{basePlugin: newBase("a")}
	a.closeOrder = &closed

	slot := newTestSlot(t, a)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := slot.Init(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Init 应返回 context.Canceled，实际 %v", err)
	}
	if len(closed) != 0 {
		t.Fatalf("ctx 取消时不应初始化插件，实际关闭 %v", closed)
	}
}

// TestPluginSlotCloseErrorContinues 验证单个插件 Close 失败后继续关闭剩余插件。
func TestPluginSlotCloseErrorContinues(t *testing.T) {
	var closed []string
	a := &readyPlugin{basePlugin: newBase("a")}
	a.closeOrder = &closed
	b := &readyPlugin{basePlugin: newBase("b")}
	b.closeErr = errors.New("close failed")
	b.closeOrder = &closed

	slot := newTestSlot(t, a, b)
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	err := slot.Close(context.Background())
	if err == nil || !strings.Contains(err.Error(), "b") {
		t.Fatalf("Close 应返回失败插件错误，实际 %v", err)
	}
	if want := []string{"b", "a"}; !slices.Equal(closed, want) {
		t.Fatalf("关闭顺序 = %v, want %v", closed, want)
	}
}

// TestPluginSlotClosePanicContinues 验证 Close panic 转为错误后继续关闭剩余插件。
func TestPluginSlotClosePanicContinues(t *testing.T) {
	var closed []string
	a := &readyPlugin{basePlugin: newBase("a")}
	a.closeOrder = &closed
	b := &basePlugin{Base: NewBase("b"), closePanic: true}

	slot := newTestSlot(t, a, b)
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	err := slot.Close(context.Background())
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("Close panic 应转为错误，实际 %v", err)
	}
	if want := []string{"a"}; !slices.Equal(closed, want) {
		t.Fatalf("panic 后应继续关闭前序插件，关闭顺序 = %v, want %v", closed, want)
	}
}

// TestPluginSlotDisabledNotClosed 验证 ErrDisabled 插件不参与 Close。
func TestPluginSlotDisabledNotClosed(t *testing.T) {
	var closed []string
	p := &readyPlugin{basePlugin: newBase("disabled")}
	p.initErr = ErrDisabled
	p.closeOrder = &closed

	slot := newTestSlot(t, p)
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	if slot.Started("disabled") {
		t.Fatal("ErrDisabled 插件不应被视为已初始化")
	}
	if err := slot.Close(context.Background()); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}
	if len(closed) != 0 {
		t.Fatalf("ErrDisabled 插件不应被关闭，实际 %v", closed)
	}
}

// TestPluginSlotDependencyLookup 验证插件可在 Init 中查找前序插件组件。
func TestPluginSlotDependencyLookup(t *testing.T) {
	provider := &providerPlugin{basePlugin: newBase("provider"), value: "ready"}
	consumer := &consumerPlugin{basePlugin: newBase("consumer")}

	slot := newTestSlot(t, provider, consumer)
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}

	if consumer.got != "ready" {
		t.Fatalf("consumer 读取依赖 = %q, want %q", consumer.got, "ready")
	}
}

// TestPluginSlotNil 验证 nil PluginSlot 的生命周期方法安全。
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
	if err := slot.Init(context.Background()); err != nil {
		t.Fatalf("nil slot Init 应返回 nil，实际 %v", err)
	}
	if err := slot.Close(context.Background()); err != nil {
		t.Fatalf("nil slot Close 应返回 nil，实际 %v", err)
	}
}
