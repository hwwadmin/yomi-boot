// Package idempotency 定义 sys_biz 幂等表模型
//
// @author zwei
package idempotency

import (
	"time"

	"gorm.io/gorm"
)

// 处理状态
const (
	StatusProcessing int8 = 1 // 处理中
	StatusCompleted  int8 = 2 // 已完成
)

// SysBiz 映射 sys_biz 幂等表
type SysBiz struct {
	ID            int64          `gorm:"column:id;primaryKey" json:"id,string"`                             // ID 是主键
	BizID         string         `gorm:"column:biz_id;size:100;not null;uniqueIndex:uk_biz,priority:1"`     // BizID 是业务幂等键
	BizSubID      string         `gorm:"column:biz_sub_id;size:100;not null;uniqueIndex:uk_biz,priority:2"` // BizSubID 是业务子幂等键
	OperationType string         `gorm:"column:operation_type;size:32;not null"`                            // OperationType 是操作类型
	Status        int8           `gorm:"column:status;not null;default:1"`                                  // Status 是处理状态：1-处理中，2-已完成
	ResponseData  []byte         `gorm:"column:response_data;type:json"`                                    // ResponseData 是响应数据快照
	CreateTime    time.Time      `gorm:"column:create_time;autoCreateTime" json:"createTime"`               // CreateTime 是创建时间
	CreateBy      *int64         `gorm:"column:create_by" json:"createBy,omitempty"`                        // CreateBy 是创建人
	UpdateTime    time.Time      `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`               // UpdateTime 是更新时间
	UpdateBy      *int64         `gorm:"column:update_by" json:"updateBy,omitempty"`                        // UpdateBy 是更新人
	DeleteTime    gorm.DeletedAt `gorm:"column:delete_time;index" json:"deleteTime,omitempty"`              // DeleteTime 是软删除时间
	DeleteBy      *int64         `gorm:"column:delete_by" json:"deleteBy,omitempty"`                        // DeleteBy 是删除人
}

// TableName 返回 sys_biz 幂等表名
func (SysBiz) TableName() string {
	return "sys_biz"
}
