// Package model 定义消息队列事件 wire 模型
//
// @author zwei
package model

// WireEvent 消息队列事件 JSON 结构，字段名与 Java MqEvent 保持一致
type WireEvent struct {
	EventID string `json:"eventId"`
	Topic   string `json:"topic"`
	Payload string `json:"payload"`
}
