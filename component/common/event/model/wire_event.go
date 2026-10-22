// Package model 定义消息队列事件 wire 模型
//
// @author zwei
package model

// WireEvent 消息队列事件 JSON 结构，字段名与 Java MqEvent 保持一致。
// EventID 必须全局唯一（建议使用雪花 ID），消费侧以 (service_name, eventId) 做幂等去重。
type WireEvent struct {
	EventID string `json:"eventId"` // EventID 是全局唯一事件 ID，建议使用雪花 ID
	Topic   string `json:"topic"`   // Topic 是事件主题
	Payload string `json:"payload"` // Payload 是事件负载
}
