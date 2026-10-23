// Package builtin 维护内置插件清单：DefaultPlugins 提供按序插件列表，DefaultConfigs 提供默认配置段。
//
// 本包是 plugin 根包与 plugin/builtin/{lib} 之间的清单层：plugin 根包不得 import
// plugin/builtin/{lib}（子包依赖 plugin，会成环），因此插件清单放在本包，由 boot.MustInit 消费。
// 反向同样禁止——plugin/builtin/{lib} 不得 import 本包，否则同样成环。
//
// @author zwei
package builtin
