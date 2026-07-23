// Package idempotency 定义业务幂等键类型
//
// @author zwei
package idempotency

// Key 业务幂等键，operationType 由 usecase 内部常量提供
type Key struct {
	BizID         string
	BizSubID      string
	OperationType string
}
