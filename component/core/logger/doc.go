// Package logger 提供统一日志能力：初始化控制台与 access/info/error/debug 四文件分流、
// 文件滚动与清理、access 请求/响应采集中间件、脱敏注入与 Gin 输出重定向
//
// access 中间件对同一次 HTTP 成对写入 inbound/outbound 两条日志：请求侧字段先缓冲，
// handler 结束后按采样率决定是否落盘（status>=400 必采）。
// 脱敏由注入的 masking 服务处理（规则统一在 masking 配置段；log.mask_sensitive=false 时不脱敏），
// 未注入时日志原文直出。
//
// @author zwei
package logger
