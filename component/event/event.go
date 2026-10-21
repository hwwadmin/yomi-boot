// Package event 提供基于发件箱/收件箱模式的消息队列事件能力
//
// @author zwei
package event

import "gitee.com/hq_project/yomi-boot/component/event/model"

// Event 消息队列事件；EventID 必须全局唯一（建议使用雪花 ID）
type Event = model.WireEvent
