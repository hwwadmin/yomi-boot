package dto

const (
	// DefaultPage 是默认页码
	DefaultPage = 1
	// DefaultPageSize 是默认每页条数
	DefaultPageSize = 10
	// MaxPageSize 是每页最大条数
	MaxPageSize = 100
)

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
		p.Page = DefaultPage
	}
	if p.PageSize <= 0 {
		p.PageSize = DefaultPageSize
	}
	if p.PageSize > MaxPageSize {
		p.PageSize = MaxPageSize
	}
}

// PageOutput 表示分页载荷，由业务出参结构体内嵌，随响应 data 序列化为 {items,total}
type PageOutput[T any] struct {
	Items []T   `json:"items"` // Items 是当前页结果列表
	Total int64 `json:"total"` // Total 是满足条件的总记录数
}

// NewPageOutput 构造分页载荷，items 为 nil 时归一为空切片，保证序列化为 [] 而非 null
func NewPageOutput[T any](items []T, total int64) PageOutput[T] {
	if items == nil {
		items = []T{}
	}
	return PageOutput[T]{Items: items, Total: total}
}
