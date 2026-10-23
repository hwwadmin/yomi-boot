// 插件生命周期接口与运行时状态。

package plugin

import "context"

// Plugin 资源生命周期接口；插件自包含其创建的组件，并在 Close 中自行释放。
type Plugin interface {
	Name() string
	Started() bool
	Init(slot *PluginSlot)
	Close()
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
