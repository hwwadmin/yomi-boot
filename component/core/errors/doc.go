// Package errors 基于 github.com/pkg/errors 与标准库提供错误能力：哨兵、拍栈、包装与判等
//
// 业务异常（对外携带 code）见 BizError；框架库请使用各包自行维护的简单哨兵 error，
// 不要使用 BizError 定义框架错误。
//
// @author zwei
package errors
