// Package response 定义 HTTP 响应编码器接口（Encoder）及其进程级全局注册
//
// 字段形状、成败语义与 HTTP 状态码均由实现决定；常规启动经 boot.Options.ResponseEncoder 注册，
// 为空时注册 DefaultEncoder；低层启动路径须在 main 中显式 MustRegister，禁止包 init 静默注册。
//
// @author zwei
package response
