// Package event 提供基于发件箱/收件箱模式的消息队列事件能力
//
// @author zwei
package event

import "yt-boot/errors"

// ErrDuplicatePublish 表示同一 service 下 eventId 重复发布
var ErrDuplicatePublish = errors.NewNoStack("event: 消息队列事件重复发布")

// ErrEventInvalid 表示事件参数不合法
var ErrEventInvalid = errors.NewNoStack("event: 事件参数不合法")
