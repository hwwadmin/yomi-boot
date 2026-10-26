// 定义向量库门面与公共类型。
//
// @author zwei

package vectordb

import "context"

// Store 向量库统一门面；同一实例可并发调用。集合名每次传入，向量维度锁定在集合上，不同集合可不同。
type Store interface {
	// CreateCollection 创建空集合；dimension 为向量维度（必填且 >0）；已存在返回 ErrAlreadyExists
	CreateCollection(ctx context.Context, name string, dimension int) error
	// DropCollection 删除集合；不存在时幂等成功
	DropCollection(ctx context.Context, name string) error
	// ReplaceCollection 用 records 完整覆盖集合内容；不存在则按 dimension 创建；已存在且维度
	// 不一致返回 ErrInvalidArgument；空切片保留空集合；失败时不得把旧快照清成不可恢复
	ReplaceCollection(ctx context.Context, name string, dimension int, records []Record) error
	// HasCollection 查询集合是否存在；不存在返回 false, nil
	HasCollection(ctx context.Context, name string) (bool, error)
	// EnsureCollection 确保集合存在；不存在时按 dimension 创建，已存在且维度不一致返回 ErrInvalidArgument
	EnsureCollection(ctx context.Context, name string, dimension int) error

	// Upsert 按 ID 覆盖写入；空切片非法；集合不存在返回 ErrNotFound
	Upsert(ctx context.Context, collection string, records []Record) error
	// Get 按 ID 取回；只返回命中；空 ids 非法；集合不存在返回 ErrNotFound；内部按批查询，调用方无需分批
	Get(ctx context.Context, collection string, ids []string) ([]Record, error)
	// ListIDs 按元数据等值 AND 列出主键；filter 至少 1 个键；集合不存在返回 ErrNotFound；无命中返回空切片
	ListIDs(ctx context.Context, collection string, filter map[string]string) ([]string, error)
	// Delete 按 ID 删除；缺失 ID 幂等成功；空 ids 非法；集合不存在返回 ErrNotFound
	Delete(ctx context.Context, collection string, ids []string) error
	// DeleteByFilter 按元数据等值 AND 删除；filter 至少 1 个键；集合不存在返回 ErrNotFound
	DeleteByFilter(ctx context.Context, collection string, filter map[string]string) error
	// ReplaceByFilter 按过滤键覆盖写入并清理旧记录：先 Upsert records，再删除 filter 命中
	// 但不在 records 中的主键；records 为空时只清理；filter 至少 1 个键；集合不存在返回
	// ErrNotFound；同一过滤键的替换在本实例内串行，records 应属于 filter 命中的集合
	ReplaceByFilter(ctx context.Context, collection string, filter map[string]string, records []Record) error
	// Search 向量检索；Text 为空走纯向量，非空走向量 + BM25 混合检索（RRF 融合）；
	// 无命中返回空切片；集合不存在返回 ErrNotFound；Hit 不含 Vector
	Search(ctx context.Context, collection string, q SearchQuery) ([]Hit, error)

	// Close 释放底层资源；可重复调用
	Close() error
}

// Record 一条待写入或按 ID 取回的向量记录
type Record struct {
	ID       string            // ID 是主键；trim 后非空，最长 maxIDLen 字节
	Vector   []float32         // Vector 长度必须等于集合维度
	Text     string            // Text 可选；BM25 词法检索的原文，空串表示该条不参与词法命中；最长 maxTextLen 字节
	Metadata map[string]string // Metadata 可选；键 trim 后不得为空
}

// SearchQuery 向量检索入参
type SearchQuery struct {
	Vector []float32         // Vector 长度必须等于集合维度
	Text   string            // Text 可选；非空时启用向量 + BM25 混合检索，BM25 命中 Record.Text；最长 maxTextLen 字节
	TopK   int               // TopK 必须 > 0 且不超过 MaxTopK
	Filter map[string]string // Filter 可选；nil/空表示不过滤；全键等值 AND
}

// Hit 一条检索命中；Score 越大越相似（L2 已映射）
type Hit struct {
	ID       string            // ID 是主键
	Score    float32           // Score 越大越相似；纯向量与混合检索的 Score 不可横比（混合为 RRF 融合分）
	Metadata map[string]string // Metadata 为写入时的标量副本
}
