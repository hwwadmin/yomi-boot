// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

import (
	"time"

	"gitee.com/hq_project/yomi-boot/component/db"
)

// SysEventOutboxDlq 映射 sys_event_outbox_dlq 发件箱死信表
type SysEventOutboxDlq struct {
	ServiceName    string     `gorm:"column:service_name;size:100;not null"`
	EventOutboxID  int64      `gorm:"column:event_outbox_id;not null"`
	ErrorMsg       string     `gorm:"column:error_msg;type:text"`
	RetryCount     int        `gorm:"column:retry_count;not null;default:0"`
	RetryLastTime  *time.Time `gorm:"column:retry_last_time"`
	RetryAfterTime *time.Time `gorm:"column:retry_after_time"`
	db.BaseModel
}

// TableName 返回发件箱死信表名
func (SysEventOutboxDlq) TableName() string {
	return "sys_event_outbox_dlq"
}
