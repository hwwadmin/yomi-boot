// Package main 是 yomi-boot 组件库的快速启动示例。
//
// 示例不含任何业务：只通过 boot.Run 完成配置加载、内置插件链初始化、默认中间件
// 装配与 HTTP 服务生命周期编排，用于验证组件库能否在最小配置下独立跑通。
//
// 配置文件按进程工作目录解析，运行前进入本目录：
//
//	cd quick-starter-demo && go run .
//
// 运行环境由 YOMI_CONFIG 选择（local/dev/prod，默认 dev）。
//
// @author zwei
package main

import (
	"gitee.com/hq_project/yomi-boot/boot"
	"gitee.com/hq_project/yomi-boot/component/common/router"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// envPrefix 是传给 boot.Run 的环境变量前缀。
const envPrefix = "YOMI_DEMO"

func main() {
	boot.Run(envPrefix, wire)
}

// wire 是路由装配入口；示例无业务，返回空注册器列表。
func wire(*plugin.PluginSlot) []router.Registrar {
	return nil
}
