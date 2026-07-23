// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

// OutboxStatus 发件箱事件状态
type OutboxStatus string

// 发件箱状态枚举
const (
	OutboxStatusPending     OutboxStatus = "PENDING"
	OutboxStatusFetched     OutboxStatus = "FETCHED"
	OutboxStatusSentSuccess OutboxStatus = "SENT_SUCCESS"
	OutboxStatusSentFail    OutboxStatus = "SENT_FAIL"
)
