// Package db 提供数据库连接初始化与通用持久化模型
//
// @author zwei
package db

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel 表示基础持久化模型，内嵌后自动获得主键、审计字段与软删除能力
type BaseModel struct {
	ID         int64          `gorm:"column:id;primaryKey" json:"id,string"`                // 主键
	CreateTime time.Time      `gorm:"column:create_time;autoCreateTime" json:"createTime"`  // 创建时间
	CreateBy   *int64         `gorm:"column:create_by" json:"createBy,omitempty"`           // 创建人
	UpdateTime time.Time      `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`  // 更新时间
	UpdateBy   *int64         `gorm:"column:update_by" json:"updateBy,omitempty"`           // 更新人
	DeleteTime gorm.DeletedAt `gorm:"column:delete_time;index" json:"deleteTime,omitempty"` // 软删除时间
	DeleteBy   *int64         `gorm:"column:delete_by" json:"deleteBy,omitempty"`           // 删除人
}
