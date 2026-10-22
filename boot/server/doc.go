// Package server 提供 HTTP 服务构建与中间件装配。
//
// 本包只处理 HTTP 侧：中间件链、gin 引擎、http.Server 与 lifecycle.App；
// 基础设施初始化在 boot（MustInit），二者由 boot.RunWithOptions 串联。
//
// @author zwei
package server
