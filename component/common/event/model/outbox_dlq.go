// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

import (
	"time"

	"gorm.io/gorm"
)

// SysEventOutboxDlq 映射 sys_event_outbox_dlq 发件箱死信表
type SysEventOutboxDlq struct {
	ID             int64          `gorm:"column:id;primaryKey" json:"id,string"` // ID 是主键
	ServiceName    string         `gorm:"column:service_name;size:100;not null"`
	EventOutboxID  int64          `gorm:"column:event_outbox_id;not null"`
	ErrorMsg       string         `gorm:"column:error_msg;type:text"`
	RetryCount     int            `gorm:"column:retry_count;not null;default:0"`
	RetryLastTime  *time.Time     `gorm:"column:retry_last_time"`
	RetryAfterTime *time.Time     `gorm:"column:retry_after_time"`
	CreateTime     time.Time      `gorm:"column:create_time;autoCreateTime" json:"createTime"`  // CreateTime 是创建时间
	CreateBy       *int64         `gorm:"column:create_by" json:"createBy,omitempty"`           // CreateBy 是创建人
	UpdateTime     time.Time      `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`  // UpdateTime 是更新时间
	UpdateBy       *int64         `gorm:"column:update_by" json:"updateBy,omitempty"`           // UpdateBy 是更新人
	DeleteTime     gorm.DeletedAt `gorm:"column:delete_time;index" json:"deleteTime,omitempty"` // DeleteTime 是软删除时间
	DeleteBy       *int64         `gorm:"column:delete_by" json:"deleteBy,omitempty"`           // DeleteBy 是删除人
}

// TableName 返回发件箱死信表名
func (SysEventOutboxDlq) TableName() string {
	return "sys_event_outbox_dlq"
}
