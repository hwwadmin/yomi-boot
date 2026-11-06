// Package idempotency 定义业务幂等键类型
//
// @author zwei
package idempotency

import "regexp"

// bizPartPattern 是 biz_id、biz_sub_id 的格式：1～100 位字母、数字、下划线或连字符。
// 与 sys_biz 这两列的 varchar(100) 一致，并且不含冒号。
// 结果缓存和分布式锁按「前缀 + biz_id + : + biz_sub_id」拼接，冒号只能做分隔符。
var bizPartPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

// Key 业务幂等键。
// BizID、BizSubID 为 1～100 位字母、数字、下划线或连字符。OperationType 由 usecase 内部常量提供，不参与查重。
type Key struct {
	BizID         string
	BizSubID      string
	OperationType string
}

// validBizPart 报告一段业务键是否符合 bizPartPattern
func validBizPart(s string) bool {
	return bizPartPattern.MatchString(s)
}
