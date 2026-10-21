// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

import "gitee.com/hq_project/yomi-boot/component/db"

// SysEventInbox 映射 sys_event_inbox 收件箱表
type SysEventInbox struct {
	ServiceName string `gorm:"column:service_name;size:100;not null"` // ServiceName 是消费方服务名
	EventID     string `gorm:"column:event_id;size:100;not null"`     // EventID 是全局唯一事件 ID
	Topic       string `gorm:"column:topic;size:200;not null"`        // Topic 是事件主题
	Payload     string `gorm:"column:payload;type:longtext"`          // Payload 是事件负载
	db.BaseModel
}

// TableName 返回收件箱表名
func (SysEventInbox) TableName() string {
	return "sys_event_inbox"
}
