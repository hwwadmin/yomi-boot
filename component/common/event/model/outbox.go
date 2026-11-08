// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

import (
	"time"

	"gorm.io/gorm"
)

// SysEventOutbox 映射 t_sys_event_outbox 发件箱表
type SysEventOutbox struct {
	ID          int64          `gorm:"column:id;primaryKey" json:"id,string"` // ID 是主键
	ServiceName string         `gorm:"column:service_name;size:100;not null"` // ServiceName 是发布方服务名
	EventID     string         `gorm:"column:event_id;size:100;not null"`     // EventID 是全局唯一事件 ID
	Topic       string         `gorm:"column:topic;size:200;not null"`        // Topic 是事件主题
	Payload     string         `gorm:"column:payload;type:longtext"`          // Payload 是事件负载
	Status      OutboxStatus   `gorm:"column:status;size:20;not null;default:PENDING"`
	FetchTime   *time.Time     `gorm:"column:fetch_time"`
	SentTime    *time.Time     `gorm:"column:sent_time"`
	CreateTime  time.Time      `gorm:"column:create_time;autoCreateTime" json:"createTime"`  // CreateTime 是创建时间
	CreateBy    *int64         `gorm:"column:create_by" json:"createBy,omitempty"`           // CreateBy 是创建人
	UpdateTime  time.Time      `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`  // UpdateTime 是更新时间
	UpdateBy    *int64         `gorm:"column:update_by" json:"updateBy,omitempty"`           // UpdateBy 是更新人
	DeleteTime  gorm.DeletedAt `gorm:"column:delete_time;index" json:"deleteTime,omitempty"` // DeleteTime 是软删除时间
	DeleteBy    *int64         `gorm:"column:delete_by" json:"deleteBy,omitempty"`           // DeleteBy 是删除人
}

// TableName 返回发件箱表名
func (SysEventOutbox) TableName() string {
	return "t_sys_event_outbox"
}
