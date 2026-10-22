// Package plugin 提供插件资源生命周期、进程阶段 hook 与初始化上下文。
//
// 内置插件与服务自定义插件均实现 Plugin 接口；需要参与进程阶段的插件按需实现
// ReadyHook / DrainHook，由 lifecycle 在对应阶段统一调用。
//
// Host 是插件 Init 阶段的共享上下文，Services 是进程启动后业务装配可见的能力集合。
//
// @author zwei
package plugin
