// Package plugin 提供插件生命周期、统一插件插槽与进程阶段 hook。
//
// 内置插件与服务自定义插件均实现 Plugin 接口；需要参与进程阶段的插件按需实现
// ReadyHook / DrainHook，由 lifecycle 在对应阶段统一调用。
//
// PluginSlot 是唯一的插件容器：除 Config 与单个插件的关闭等待上限外只保存有序插件链，
// 负责按序 Init、收集阶段 hook 与逆序 Close。Close 对每个插件限时等待，超时记 Warn 后
// 继续关闭前序插件，避免单个插件卡住拖住整个进程。插件对象自包含其创建的组件，跨插件
// 依赖通过 slot.Plugin / slot.MustPlugin 取到前序插件后调用其访问器，不再使用共享上下文。
//
// PluginOptions 汇总服务自定义插件与自定义配置段，作为插件链的扩展入口。
//
// @author zwei
package plugin
