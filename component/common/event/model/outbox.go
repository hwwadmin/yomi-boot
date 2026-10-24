// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

import (
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/orm"
)

// SysEventOutbox 映射 sys_event_outbox 发件箱表
type SysEventOutbox struct {
	ServiceName string       `gorm:"column:service_name;size:100;not null"` // ServiceName 是发布方服务名
	EventID     string       `gorm:"column:event_id;size:100;not null"`     // EventID 是全局唯一事件 ID
	Topic       string       `gorm:"column:topic;size:200;not null"`        // Topic 是事件主题
	Payload     string       `gorm:"column:payload;type:longtext"`          // Payload 是事件负载
	Status      OutboxStatus `gorm:"column:status;size:20;not null;default:PENDING"`
	FetchTime   *time.Time   `gorm:"column:fetch_time"`
	SentTime    *time.Time   `gorm:"column:sent_time"`
	orm.BaseModel
}

// TableName 返回发件箱表名
func (SysEventOutbox) TableName() string {
	return "sys_event_outbox"
}
