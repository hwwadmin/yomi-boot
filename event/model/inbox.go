// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

import "yt-boot/db"

// SysEventInbox 映射 sys_event_inbox 收件箱表
type SysEventInbox struct {
	ServiceName string `gorm:"column:service_name;size:100;not null"`
	EventID     string `gorm:"column:event_id;size:100;not null"`
	Topic       string `gorm:"column:topic;size:200;not null"`
	Payload     string `gorm:"column:payload;type:longtext"`
	db.BaseModel
}

// TableName 返回收件箱表名
func (SysEventInbox) TableName() string {
	return "sys_event_inbox"
}
