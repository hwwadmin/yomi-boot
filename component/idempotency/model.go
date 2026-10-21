// Package idempotency 定义 sys_biz 幂等表模型
//
// @author zwei
package idempotency

import "gitee.com/hq_project/yomi-boot/component/db"

// 处理状态
const (
	StatusProcessing int8 = 1 // 处理中
	StatusCompleted  int8 = 2 // 已完成
)

// SysBiz 映射 sys_biz 幂等表
type SysBiz struct {
	BizID         string `gorm:"column:biz_id;size:100;not null"`        // BizID 是业务幂等键
	BizSubID      string `gorm:"column:biz_sub_id;size:100;not null"`    // BizSubID 是业务子幂等键
	OperationType string `gorm:"column:operation_type;size:32;not null"` // OperationType 是操作类型
	Status        int8   `gorm:"column:status;not null;default:1"`       // Status 是处理状态：1-处理中，2-已完成
	ResponseData  []byte `gorm:"column:response_data;type:json"`         // ResponseData 是响应数据快照
	db.BaseModel
}

// TableName 返回 sys_biz 幂等表名
func (SysBiz) TableName() string {
	return "sys_biz"
}
