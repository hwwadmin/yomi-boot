// PluginSlot 定义统一插件插槽：保存配置与有序插件链，并负责整条链的生命周期。

package plugin

import (
	"context"
	"slices"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// PluginSlot 是统一的插件插槽。
//
// 除 Config 外只保存有序插件链；插件对象自包含其创建的组件，跨插件依赖通过
// Plugin / MustPlugin 取到前序插件后再调用其访问器。Init 完成后为只读结构。
type PluginSlot struct {
	Config *config.Config // Config 是已绑定配置（含自定义段）

	chain  []Plugin          // chain 是注册顺序的插件链；Init/ReadyHooks/DrainHooks/Close 均按它遍历
	index  map[string]Plugin // index 是插件名到插件实例的索引；仅用于按名查找
	inited bool              // inited 标记 Init 是否已执行；禁止重复 Init
	closed bool              // closed 标记 Close 是否已执行；Close 幂等，关闭后禁止 Init
}

// NewPluginSlot 创建插件插槽；插件为空、名称为空或重名时 panic（Fail Fast）
func NewPluginSlot(cfg *config.Config, plugins []Plugin) *PluginSlot {
	if cfg == nil {
		panic(errors.New("plugin: Config 不能为空"))
	}

	slot := &PluginSlot{
		Config: cfg,
		chain:  make([]Plugin, 0, len(plugins)),
		index:  make(map[string]Plugin, len(plugins)),
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

// Close 逆序关闭插件链，失败由各插件自行处理；可重复调用，未 Init 的插件必须安全无操作
func (s *PluginSlot) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	for i := len(s.chain) - 1; i >= 0; i-- {
		s.chain[i].Close()
	}
}
