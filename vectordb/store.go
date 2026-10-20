// Package vectordb 定义向量库门面与公共类型。
//
// @author zwei
package vectordb

import "context"

// Store 向量库统一门面；同一实例可并发调用。集合名每次传入，维度与度量由 Config 锁定。
type Store interface {
	// CreateCollection 创建空集合；已存在返回 ErrAlreadyExists
	CreateCollection(ctx context.Context, name string) error
	// DropCollection 删除集合；不存在时幂等成功
	DropCollection(ctx context.Context, name string) error
	// ReplaceCollection 用 records 完整覆盖集合内容；不存在则先创建；空切片保留空集合；失败时不得把旧快照清成不可恢复
	ReplaceCollection(ctx context.Context, name string, records []Record) error
	// HasCollection 查询集合是否存在；不存在返回 false, nil
	HasCollection(ctx context.Context, name string) (bool, error)

	// Upsert 按 ID 覆盖写入；空切片非法；集合不存在返回 ErrNotFound
	Upsert(ctx context.Context, collection string, records []Record) error
	// Get 按 ID 取回；只返回命中；空 ids 非法；集合不存在返回 ErrNotFound
	Get(ctx context.Context, collection string, ids []string) ([]Record, error)
	// ListIDs 按元数据等值 AND 列出主键；filter 至少 1 个键；集合不存在返回 ErrNotFound；无命中返回空切片
	ListIDs(ctx context.Context, collection string, filter map[string]string) ([]string, error)
	// Delete 按 ID 删除；缺失 ID 幂等成功；空 ids 非法；集合不存在返回 ErrNotFound
	Delete(ctx context.Context, collection string, ids []string) error
	// DeleteByFilter 按元数据等值 AND 删除；filter 至少 1 个键；集合不存在返回 ErrNotFound
	DeleteByFilter(ctx context.Context, collection string, filter map[string]string) error
	// Search 向量检索；无命中返回空切片；集合不存在返回 ErrNotFound；Hit 不含 Vector
	Search(ctx context.Context, collection string, q SearchQuery) ([]Hit, error)

	// Close 释放底层资源；可重复调用
	Close() error
}

// Record 一条待写入或按 ID 取回的向量记录
type Record struct {
	ID       string            // ID 是主键；trim 后非空，最长 maxIDLen 字节
	Vector   []float32         // Vector 长度必须等于 Config.Dimension
	Metadata map[string]string // Metadata 可选；键 trim 后不得为空
}

// SearchQuery 向量检索入参
type SearchQuery struct {
	Vector []float32         // Vector 长度必须等于 Config.Dimension
	TopK   int               // TopK 必须 > 0 且不超过 MaxTopK
	Filter map[string]string // Filter 可选；nil/空表示不过滤；全键等值 AND
}

// Hit 一条检索命中；Score 越大越相似（L2 已映射）
type Hit struct {
	ID       string            // ID 是主键
	Score    float32           // Score 越大越相似；不同 Metric 不可横比
	Metadata map[string]string // Metadata 为写入时的标量副本
}
