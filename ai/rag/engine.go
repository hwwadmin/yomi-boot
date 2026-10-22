// Engine 定义与集合生命周期。
//
// @author zwei

package rag

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"gitee.com/hq_project/yomi-boot/ai/embedding"
	"gitee.com/hq_project/yomi-boot/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// embedBatchSize 是单次向量化的文本条数上限
const embedBatchSize = 64

// indexLockStripes 是 Index 写入阶段的锁条数；同一文档命中同一条带
const indexLockStripes = 64

// Engine 检索增强引擎；同一实例可并发服务多个集合；构造后不可变，请勿复制值
type Engine struct {
	embedder        embedding.Client
	store           vectordb.Store
	defaultSplitter Splitter
	dimension       atomic.Int64
	indexLocks      [indexLockStripes]sync.Mutex
}

// EnsureCollection 确保集合存在；已存在时返回 nil
func (e *Engine) EnsureCollection(ctx context.Context, collection string) error {
	name, err := normalizeCollection(collection)
	if err != nil {
		return err
	}
	return e.ensureCollection(orBackground(ctx), name)
}

// DropCollection 删除集合；不存在时返回 nil
func (e *Engine) DropCollection(ctx context.Context, collection string) error {
	name, err := normalizeCollection(collection)
	if err != nil {
		return err
	}
	return e.store.DropCollection(orBackground(ctx), name)
}

// ensureCollection 查询并按需创建集合；并发创建撞车时视为成功
func (e *Engine) ensureCollection(ctx context.Context, collection string) error {
	has, err := e.store.HasCollection(ctx, collection)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if err := e.store.CreateCollection(ctx, collection); err != nil {
		if errors.Is(err, vectordb.ErrAlreadyExists) {
			return nil
		}
		return err
	}
	return nil
}

// orBackground 返回可用的 ctx；nil 回退 Background
func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// embed 分批向量化并校验维度与返回条数；返回向量与用量合计
func (e *Engine) embed(ctx context.Context, texts []string) ([][]float64, int, error) {
	if len(texts) == 0 {
		return nil, 0, nil
	}
	out := make([][]float64, 0, len(texts))
	tokens := 0
	for start := 0; start < len(texts); start += embedBatchSize {
		end := start + embedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		resp, err := e.embedder.Embed(ctx, embedding.Request{Inputs: texts[start:end]})
		if err != nil {
			return nil, 0, err
		}
		if resp == nil || len(resp.Vectors) != end-start {
			return nil, 0, wrapSentinel(ErrBadResponse, errors.New("ai/rag 向量化返回条数与输入不一致"))
		}
		for i := range resp.Vectors {
			if err := e.checkDimension(len(resp.Vectors[i])); err != nil {
				return nil, 0, err
			}
		}
		if resp.Usage != nil {
			tokens += resp.Usage.TotalTokens
		}
		out = append(out, resp.Vectors...)
	}
	return out, tokens, nil
}

// checkDimension 校验向量维度；未锁定则用本次结果锁定
func (e *Engine) checkDimension(n int) error {
	if n <= 0 {
		return errors.Sentinel(ErrDimensionMismatch)
	}
	locked := e.dimension.Load()
	if locked == 0 {
		e.dimension.CompareAndSwap(0, int64(n))
		locked = e.dimension.Load()
	}
	if int64(n) != locked {
		return errors.Sentinel(ErrDimensionMismatch)
	}
	return nil
}

// chunkID 拼接块主键；文档 ID 最长 120，块序号最大 4 位，总长不超过 128
func chunkID(docID string, index int) string {
	return docID + "#" + strconv.Itoa(index)
}

// toFloat32 将向量化输出转为向量库所需的 float32
func toFloat32(vec []float64) []float32 {
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(v)
	}
	return out
}

// lockDocuments 按文档获取写入锁；条带按序加锁避免死锁，返回释放函数
func (e *Engine) lockDocuments(collection string, docs []preparedDocument) func() {
	stripes := make([]int, 0, len(docs))
	seen := make(map[int]struct{}, len(docs))
	for _, doc := range docs {
		stripe := int(indexLockHash(collection, doc.doc.ID) % indexLockStripes)
		if _, ok := seen[stripe]; ok {
			continue
		}
		seen[stripe] = struct{}{}
		stripes = append(stripes, stripe)
	}
	sort.Ints(stripes)
	for _, stripe := range stripes {
		e.indexLocks[stripe].Lock()
	}
	return func() {
		for i := len(stripes) - 1; i >= 0; i-- {
			e.indexLocks[stripes[i]].Unlock()
		}
	}
}

// indexLockHash 计算集合与文档 ID 的 FNV-1a 哈希
func indexLockHash(collection, docID string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := 0; i < len(collection); i++ {
		h ^= uint32(collection[i])
		h *= prime32
	}
	h *= prime32
	for i := 0; i < len(docID); i++ {
		h ^= uint32(docID[i])
		h *= prime32
	}
	return h
}
