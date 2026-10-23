// Package plugin 提供插件生命周期、统一插件插槽与进程阶段 hook。
//
// 内置插件与服务自定义插件均实现 Plugin 接口；需要参与进程阶段的插件按需实现
// ReadyHook / DrainHook，由 lifecycle 在对应阶段统一调用。
//
// PluginSlot 是唯一的插件容器：除 Config 外只保存有序插件链与每个插件的初始化状态，
// 负责按序 Init、失败回滚、收集阶段 hook 与逆序 Close。Init / Close 接收 ctx；插件返回
// ErrDisabled 时记为跳过，不参与 hook 与 Close。插件对象自包含其创建的组件，跨插件
// 依赖通过 slot.Plugin / slot.MustPlugin 取到前序插件后调用其访问器，不再使用共享上下文。
//
// PluginOptions 汇总服务自定义插件与自定义配置段，是默认插件链的扩展入口；
// 需要完全替换插件链与配置段时使用 boot.PluginSet。
//
// @author zwei
package plugin
