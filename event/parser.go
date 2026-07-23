// Package event 提供消息队列事件解析能力
//
// @author zwei
package event

import (
	"encoding/json"
	"yomi-boot/logger"
)

// ParseAndValidate 解析并校验消息队列事件
//
// requireEventID 为 true 时 eventId 必填；校验失败时 valid=false，调用方应丢弃消息
func ParseAndValidate(message []byte, requireEventID bool) (Event, bool) {
	if len(message) == 0 {
		logger.WarnWithoutCtx("消息队列事件为空，已被抛弃")
		return Event{}, false
	}

	var evt Event
	if err := json.Unmarshal(message, &evt); err != nil {
		logger.ErrorWithoutCtx("消息队列事件反序列化异常，已被抛弃", "message", string(message), "error", err)
		return Event{}, false
	}
	if evt.Topic == "" {
		logger.ErrorWithoutCtx("消息队列事件 topic 为空，已被抛弃", "message", string(message))
		return Event{}, false
	}
	if requireEventID && evt.EventID == "" {
		logger.ErrorWithoutCtx("消息队列事件 eventId 为空，已被抛弃", "message", string(message))
		return Event{}, false
	}
	return evt, true
}
