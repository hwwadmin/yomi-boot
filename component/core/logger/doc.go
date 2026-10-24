// Package logger 提供统一日志能力：初始化控制台与 access/info/error/debug 四文件分流、
// 文件滚动与清理、access 请求/响应采集中间件、敏感字段脱敏与 Gin 输出重定向
//
// access 中间件对同一次 HTTP 成对写入 inbound/outbound 两条日志：请求侧字段先缓冲，
// handler 结束后按采样率决定是否落盘（status>=400 必采）。
// 脱敏仅由 log.sensitive_keys / log.sensitive_match 驱动，keys 为空则不脱敏。
//
// @author zwei
package logger
