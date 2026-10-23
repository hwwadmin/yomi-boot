// Package plugin 提供插件生命周期、统一插件插槽与进程阶段 hook。
//
// 内置插件与服务自定义插件均实现 Plugin 接口；需要参与进程阶段的插件按需实现
// ReadyHook / DrainHook，由 lifecycle 在对应阶段统一调用。
//
// PluginSlot 是唯一的插件容器：除 Config 外只保存有序插件链，负责按序 Init、
// 收集阶段 hook 与逆序 Close。插件对象自包含其创建的组件，跨插件依赖通过
// slot.Plugin / slot.MustPlugin 取到前序插件后调用其访问器，不再使用共享上下文。
//
// PluginOptions 汇总服务自定义插件与自定义配置段，作为插件链的扩展入口。
//
// @author zwei
package plugin
