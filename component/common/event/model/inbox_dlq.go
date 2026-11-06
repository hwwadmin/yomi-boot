// Package model 定义消息队列事件持久化模型
//
// @author zwei
package model

import (
	"time"

	"gorm.io/gorm"
)

// SysEventInboxDlq 映射 sys_event_inbox_dlq 收件箱死信表
type SysEventInboxDlq struct {
	ID             int64          `gorm:"column:id;primaryKey" json:"id,string"`                                 // ID 是主键
	ServiceName    string         `gorm:"column:service_name;size:100;not null;uniqueIndex:uk_service_event_id"` // ServiceName 是消费方服务名
	EventID        string         `gorm:"column:event_id;size:100;not null;uniqueIndex:uk_service_event_id"`     // EventID 是全局唯一事件 ID
	Topic          string         `gorm:"column:topic;size:200;not null"`                                        // Topic 是事件主题
	Payload        string         `gorm:"column:payload;type:longtext"`                                          // Payload 是事件负载
	ErrorMsg       string         `gorm:"column:error_msg;type:text"`                                            // ErrorMsg 是失败信息
	RetryCount     int            `gorm:"column:retry_count;not null;default:0"`                                 // RetryCount 是已重试次数
	RetryLastTime  *time.Time     `gorm:"column:retry_last_time"`                                                // RetryLastTime 是最后重投时间
	RetryAfterTime *time.Time     `gorm:"column:retry_after_time"`                                               // RetryAfterTime 是最早可重投时间
	CreateTime     time.Time      `gorm:"column:create_time;autoCreateTime" json:"createTime"`                   // CreateTime 是创建时间
	CreateBy       *int64         `gorm:"column:create_by" json:"createBy,omitempty"`                            // CreateBy 是创建人
	UpdateTime     time.Time      `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`                   // UpdateTime 是更新时间
	UpdateBy       *int64         `gorm:"column:update_by" json:"updateBy,omitempty"`                            // UpdateBy 是更新人
	DeleteTime     gorm.DeletedAt `gorm:"column:delete_time;index" json:"deleteTime,omitempty"`                  // DeleteTime 是软删除时间
	DeleteBy       *int64         `gorm:"column:delete_by" json:"deleteBy,omitempty"`                            // DeleteBy 是删除人
}

// TableName 返回收件箱死信表名
func (SysEventInboxDlq) TableName() string {
	return "sys_event_inbox_dlq"
}
