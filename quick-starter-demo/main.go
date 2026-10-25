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
	"gitee.com/hq_project/yomi-boot/component/core/masking"
	"gitee.com/hq_project/yomi-boot/component/core/router"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// envPrefix 是传给 boot.Run 的环境变量前缀。
const envPrefix = "YOMI_DEMO"

// init 演示业务自定义脱敏策略：须在 boot.Run 之前注册，
// masking 段即可按名引用（见 config/dev.yml 的 plate_no 规则）。
func init() {
	// plate 车牌策略：京A12345 → 京A***45；形状不符（非 7 字符）时交回引擎替换为 placeholder
	masking.MustRegisterStrategy("plate", func(value string) (string, bool) {
		runes := []rune(value)
		if len(runes) != 7 {
			return "", false
		}
		return string(runes[:2]) + "***" + string(runes[5:]), true
	})
}

func main() {
	boot.Run(envPrefix, wire)
}

// wire 是路由装配入口；示例无业务，返回空注册器列表。
func wire(*plugin.PluginSlot) []router.Registrar {
	return nil
}
