// PluginSlot 定义统一插件插槽：保存配置与有序插件链，并负责整条链的生命周期。

package plugin

import (
	"context"
	"slices"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// defaultCloseTimeout 是单个插件 Close 的默认等待上限
const defaultCloseTimeout = 10 * time.Second

// PluginSlot 是统一的插件插槽。
//
// 除 Config 与单个插件的关闭等待上限外只保存有序插件链；插件对象自包含其创建的组件，
// 跨插件依赖通过 Plugin / MustPlugin 取到前序插件后再调用其访问器。Init 完成后插件链为只读结构。
type PluginSlot struct {
	Config *config.Config // Config 是已绑定配置（含自定义段）

	chain        []Plugin          // chain 是注册顺序的插件链；Init/ReadyHooks/DrainHooks/Close 均按它遍历
	index        map[string]Plugin // index 是插件名到插件实例的索引；仅用于按名查找
	inited       bool              // inited 标记 Init 是否已执行；禁止重复 Init
	closed       bool              // closed 标记 Close 是否已执行；Close 幂等，关闭后禁止 Init
	closeTimeout time.Duration     // closeTimeout 是单个插件 Close 的等待上限；<=0 时使用默认值
}

// NewPluginSlot 创建插件插槽；插件为空、名称为空或重名时 panic（Fail Fast）
func NewPluginSlot(cfg *config.Config, plugins []Plugin) *PluginSlot {
	if cfg == nil {
		panic(errors.New("plugin: Config 不能为空"))
	}

	slot := &PluginSlot{
		Config:       cfg,
		chain:        make([]Plugin, 0, len(plugins)),
		index:        make(map[string]Plugin, len(plugins)),
		closeTimeout: defaultCloseTimeout,
	}
	for _, p := range plugins {
		if p == nil {
			panic(errors.New("plugin: 插件不能为空"))
		}
		name := strings.TrimSpace(p.Name())
		if name == "" {
			panic(errors.New("plugin: 插件名称不能为空"))
		}
		if _, exists := slot.index[name]; exists {
			panic(errors.Newf("plugin: 插件名称重复: %s", name))
		}
		slot.chain = append(slot.chain, p)
		slot.index[name] = p
	}
	return slot
}

// Init 按插件链顺序执行 Init；失败时由插件 panic，进程直接退出
func (s *PluginSlot) Init() {
	if s == nil {
		return
	}
	if s.closed {
		panic(errors.New("plugin: PluginSlot 已关闭，不能 Init"))
	}
	if s.inited {
		panic(errors.New("plugin: PluginSlot 不能重复 Init"))
	}
	s.inited = true
	for _, p := range s.chain {
		p.Init(s)
	}
}

// Plugin 按名称查找插件；插件未注册时返回 nil, false
func (s *PluginSlot) Plugin(name string) (Plugin, bool) {
	if s == nil {
		return nil, false
	}
	p, ok := s.index[strings.TrimSpace(name)]
	return p, ok
}

// MustPlugin 按名称查找插件；插件未注册时 panic
func (s *PluginSlot) MustPlugin(name string) Plugin {
	p, ok := s.Plugin(name)
	if !ok {
		panic(errors.Newf("plugin: 未注册插件: %s", name))
	}
	return p
}

// Started 返回指定名称的插件是否已实际启动
func (s *PluginSlot) Started(name string) bool {
	p, ok := s.Plugin(name)
	return ok && p.Started()
}

// Plugins 按链顺序返回插件副本
func (s *PluginSlot) Plugins() []Plugin {
	if s == nil {
		return nil
	}
	return slices.Clone(s.chain)
}

// ReadyHooks 按 Init 顺序返回已启动插件的就绪钩子
func (s *PluginSlot) ReadyHooks() []func(context.Context) error {
	if s == nil {
		return nil
	}
	var hooks []func(context.Context) error
	for _, p := range s.chain {
		if !p.Started() {
			continue
		}
		if h, ok := p.(ReadyHook); ok {
			hooks = append(hooks, h.OnReady)
		}
	}
	return hooks
}

// DrainHooks 按 Init 逆序返回已启动插件的排空钩子
func (s *PluginSlot) DrainHooks() []func(context.Context) error {
	if s == nil {
		return nil
	}
	var hooks []func(context.Context) error
	for i := len(s.chain) - 1; i >= 0; i-- {
		p := s.chain[i]
		if !p.Started() {
			continue
		}
		if h, ok := p.(DrainHook); ok {
			hooks = append(hooks, h.OnDraining)
		}
	}
	return hooks
}

// SetCloseTimeout 设置关闭单个插件时的等待上限；<=0 时使用默认值（10 秒）。
// 须在 Close 之前调用，通常由 boot 按停机预算统一设置
func (s *PluginSlot) SetCloseTimeout(timeout time.Duration) {
	if s == nil {
		return
	}
	s.closeTimeout = timeout
}

// Close 逆序关闭插件链：每个插件最多等待 closeTimeout，超时记 Warn 后继续关闭前序插件；
// 可重复调用，未 Init 的插件必须安全无操作
func (s *PluginSlot) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	timeout := s.closeTimeout
	if timeout <= 0 {
		timeout = defaultCloseTimeout
	}
	for i := len(s.chain) - 1; i >= 0; i-- {
		p := s.chain[i]
		if !closePlugin(p, timeout) {
			logger.WarnWithoutCtx("插件关闭超时，继续关闭前序插件", "plugin", p.Name(), "timeout", timeout)
		}
	}
}

// closePlugin 在 timeout 内等待插件 Close 返回，返回 false 表示超时
func closePlugin(p Plugin, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Close()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}
