// Package dto 提供跨模块复用的传输结构定义
//
// @author zwei
package dto

import "yomi-boot/shared"

// PageInput 表示分页查询输入，嵌入 usecase 入参结构体使用
type PageInput struct {
	Page     int `json:"page"`     // Page 是页码，从 1 开始
	PageSize int `json:"pageSize"` // PageSize 是每页条数
}

// ValiPage 校验分页参数，填充默认值并限制每页最大条数
func (p *PageInput) ValiPage() {
	if p == nil {
		return
	}
	if p.Page <= 0 {
		p.Page = shared.DefaultPage
	}
	if p.PageSize <= 0 {
		p.PageSize = shared.DefaultPageSize
	}
	if p.PageSize > shared.MaxPageSize {
		p.PageSize = shared.MaxPageSize
	}
}

// PageOutput 表示分页查询输出，包装 usecase 出参结构体使用
type PageOutput[T any] struct {
	Items []T   // Items 是当前页结果列表
	Total int64 // Total 是满足条件的总记录数
}
