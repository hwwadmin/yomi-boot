// 插件生命周期接口与运行时状态。

package plugin

import "context"

// Plugin 资源生命周期接口；应用层从 *Services 读取各能力库具体类型
type Plugin interface {
	Name() string
	Started() bool
	Init(host *Host)
	Close(host *Host)
}

// ReadyHook 在 HTTP 监听成功、服务宣告就绪前执行；失败会导致启动失败
type ReadyHook interface {
	OnReady(ctx context.Context) error
}

// DrainHook 在 HTTP 停止接收流量前执行；逆序调用，失败只记录日志
type DrainHook interface {
	OnDraining(ctx context.Context) error
}

// PluginState 插件名称与启动状态，默认未启动（started=false），Init 实际启动后置 true
type PluginState struct {
	name    string
	started bool
}

// NewPluginState 创建插件状态记录器
func NewPluginState(name string) PluginState {
	return PluginState{name: name}
}

// Name 返回插件名称
func (s *PluginState) Name() string {
	return s.name
}

// Started 返回插件是否已成功启动
func (s *PluginState) Started() bool {
	return s.started
}

// SetStarted 标记插件为已启动状态
func (s *PluginState) SetStarted() {
	s.started = true
}

// Runtime 记录已 Init 的插件，供进程退出时逆序释放资源
type Runtime struct {
	host    *Host
	started []Plugin
}

// Run 按顺序 Init 插件；任一 Init panic 则进程直接退出
func Run(host *Host, plugins []Plugin) *Runtime {
	rt := &Runtime{host: host}
	for _, p := range plugins {
		p.Init(host)
		rt.started = append(rt.started, p)
	}
	return rt
}

// Started 返回指定名称的插件是否已实际启动（Init 成功且 Started()==true）
func (rt *Runtime) Started(name string) bool {
	if rt == nil {
		return false
	}
	for _, p := range rt.started {
		if p.Name() == name {
			return p.Started()
		}
	}
	return false
}

// ReadyHooks 按 Init 顺序返回已启动插件的就绪钩子
func (rt *Runtime) ReadyHooks() []func(context.Context) error {
	if rt == nil {
		return nil
	}
	var hooks []func(context.Context) error
	for _, p := range rt.started {
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
func (rt *Runtime) DrainHooks() []func(context.Context) error {
	if rt == nil {
		return nil
	}
	var hooks []func(context.Context) error
	for i := len(rt.started) - 1; i >= 0; i-- {
		p := rt.started[i]
		if !p.Started() {
			continue
		}
		if h, ok := p.(DrainHook); ok {
			hooks = append(hooks, h.OnDraining)
		}
	}
	return hooks
}

// Close 逆序释放已 Init 的插件资源，失败静默跳过
func (rt *Runtime) Close() {
	if rt == nil {
		return
	}
	for i := len(rt.started) - 1; i >= 0; i-- {
		rt.started[i].Close(rt.host)
	}
	rt.started = nil
}
