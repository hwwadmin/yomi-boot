// 分页查询辅助
//
// @author zwei

package orm

import (
	"gitee.com/hq_project/yomi-boot/component/core/shared"

	"gorm.io/gorm"
)

// ApplyPage 应用分页与结构化排序
// page 从 1 开始；若 page <= 0 则默认为 1
// pageSize 若 <= 0 则默认为 10，若超过 shared.MaxPageSize 则截断
func ApplyPage(query *gorm.DB, page, pageSize int, orders ...OrderClause) (*gorm.DB, error) {
	if page <= 0 {
		page = shared.DefaultPage
	}
	if pageSize <= 0 {
		pageSize = shared.DefaultPageSize
	}
	if pageSize > shared.MaxPageSize {
		pageSize = shared.MaxPageSize
	}
	offset := (page - 1) * pageSize

	paged, err := applyOrders(query, orders...)
	if err != nil {
		return nil, err
	}
	return paged.Limit(pageSize).Offset(offset), nil
}
