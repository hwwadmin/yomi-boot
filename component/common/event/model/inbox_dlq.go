// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

import (
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db"
)

// SysEventInboxDlq 映射 sys_event_inbox_dlq 收件箱死信表
type SysEventInboxDlq struct {
	ServiceName    string     `gorm:"column:service_name;size:100;not null;uniqueIndex:uk_service_event_id"` // ServiceName 是消费方服务名
	EventID        string     `gorm:"column:event_id;size:100;not null;uniqueIndex:uk_service_event_id"`     // EventID 是全局唯一事件 ID
	Topic          string     `gorm:"column:topic;size:200;not null"`                                        // Topic 是事件主题
	Payload        string     `gorm:"column:payload;type:longtext"`                                          // Payload 是事件负载
	ErrorMsg       string     `gorm:"column:error_msg;type:text"`                                            // ErrorMsg 是失败信息
	RetryCount     int        `gorm:"column:retry_count;not null;default:0"`                                 // RetryCount 是已重试次数
	RetryLastTime  *time.Time `gorm:"column:retry_last_time"`                                                // RetryLastTime 是最后重投时间
	RetryAfterTime *time.Time `gorm:"column:retry_after_time"`                                               // RetryAfterTime 是最早可重投时间
	db.BaseModel
}

// TableName 返回收件箱死信表名
func (SysEventInboxDlq) TableName() string {
	return "sys_event_inbox_dlq"
}
