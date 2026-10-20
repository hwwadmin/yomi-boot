// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

import (
	"time"

	"gitee.com/hq_project/yomi-boot/db"
)

// SysEventInboxDlq 映射 sys_event_inbox_dlq 收件箱死信表
type SysEventInboxDlq struct {
	ServiceName    string     `gorm:"column:service_name;size:100;not null;uniqueIndex:uk_service_event_id"`
	EventID        string     `gorm:"column:event_id;size:100;not null;uniqueIndex:uk_service_event_id"`
	Topic          string     `gorm:"column:topic;size:200;not null"`
	Payload        string     `gorm:"column:payload;type:longtext"`
	ErrorMsg       string     `gorm:"column:error_msg;type:text"`
	RetryCount     int        `gorm:"column:retry_count;not null;default:0"`
	RetryLastTime  *time.Time `gorm:"column:retry_last_time"`
	RetryAfterTime *time.Time `gorm:"column:retry_after_time"`
	db.BaseModel
}

// TableName 返回收件箱死信表名
func (SysEventInboxDlq) TableName() string {
	return "sys_event_inbox_dlq"
}
