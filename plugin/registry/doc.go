// Package registry 维护内置插件的启动顺序与默认配置段注册表。
//
// 本包是 plugin 根包与 plugin/builtin 之间唯一允许反向依赖的清单层：
// plugin 根包不得 import plugin/builtin（会与子包的 plugin 依赖成环），
// 因此插件清单集中在这里，由 boot.MustInit 消费。
//
// @author zwei
package registry
