// 内置文档版本过滤器：按文档当前版本与块总数丢弃旧块。
//
// @author zwei

package retrieval

import (
	"context"
	"strconv"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/ai/vectordb"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// DocumentFilterConfig 文档版本过滤器的元数据声明；由调用方按自身写入协议提供键名
type DocumentFilterConfig struct {
	DocumentKey  string                    // DocumentKey 是命中的文档 ID 键；必填
	IndexKey     string                    // IndexKey 是块在文档内的序号键；必填
	CountKey     string                    // CountKey 是文档当前版本块总数键；必填
	VersionKey   string                    // VersionKey 是文档写入版本键；必填
	ContentKey   string                    // ContentKey 是块文本键；可选；非空时丢弃缺少该键的命中
	FirstChunkID func(docID string) string // FirstChunkID 由文档 ID 推导首块主键；必填；返回空串表示该文档不参与状态查询
}

// validate 校验过滤器配置
func (c DocumentFilterConfig) validate() error {
	if c.DocumentKey == "" || c.IndexKey == "" || c.CountKey == "" || c.VersionKey == "" || c.FirstChunkID == nil {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return nil
}

// documentFilter 文档版本过滤器实现；同一实例可并发调用
type documentFilter struct {
	store vectordb.Store
	cfg   DocumentFilterConfig
}

var _ HitFilter = (*documentFilter)(nil)

// documentState 是一次写入的版本与块总数
type documentState struct {
	total   int
	version string
}

// NewDocumentFilter 返回文档版本过滤器：丢弃不属于文档当前版本的旧块（版本不一致或
// 序号超出块总数），ContentKey 非空时同时丢弃缺少块文本的命中。
//
// 状态探测：按 FirstChunkID 推导每篇文档的首块主键并批量 Get，从首块读取当前版本与
// 块总数（同一次写入的所有块共享这两个保留值）。辅助查询失败时降级为不过滤并告警；
// 取消/超时与集合消失（vectordb.ErrNotFound）照常上抛。
func NewDocumentFilter(store vectordb.Store, cfg DocumentFilterConfig) (HitFilter, error) {
	if store == nil {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &documentFilter{store: store, cfg: cfg}, nil
}

// Filter 实现 HitFilter
func (f *documentFilter) Filter(ctx context.Context, collection string, hits []vectordb.Hit) ([]vectordb.Hit, error) {
	candidates := hits
	if f.cfg.ContentKey != "" {
		candidates = make([]vectordb.Hit, 0, len(hits))
		for _, hit := range hits {
			if strings.TrimSpace(hit.Metadata[f.cfg.ContentKey]) != "" {
				candidates = append(candidates, hit)
			}
		}
	}
	states, err := f.loadStates(ctx, collection, candidates)
	if err != nil {
		return nil, err
	}
	out := make([]vectordb.Hit, 0, len(candidates))
	for _, hit := range candidates {
		if f.isCurrent(hit, states) {
			out = append(out, hit)
		}
	}
	return out, nil
}

// isCurrent 判断命中是否属于文档当前版本；无状态信息的命中放行
func (f *documentFilter) isCurrent(hit vectordb.Hit, states map[string]documentState) bool {
	state, ok := states[strings.TrimSpace(hit.Metadata[f.cfg.DocumentKey])]
	if !ok {
		return true
	}
	if version := strings.TrimSpace(hit.Metadata[f.cfg.VersionKey]); state.version != "" && version != state.version {
		return false
	}
	index, err := strconv.Atoi(strings.TrimSpace(hit.Metadata[f.cfg.IndexKey]))
	if err != nil {
		return true
	}
	return index < state.total
}

// loadStates 读取命中文档的当前版本与块总数；辅助查询失败时降级为不过滤
func (f *documentFilter) loadStates(ctx context.Context, collection string, hits []vectordb.Hit) (map[string]documentState, error) {
	ids := make([]string, 0, len(hits))
	seen := make(map[string]struct{}, len(hits))
	for _, hit := range hits {
		docID := strings.TrimSpace(hit.Metadata[f.cfg.DocumentKey])
		if docID == "" {
			continue
		}
		if _, ok := seen[docID]; ok {
			continue
		}
		firstChunkID := f.cfg.FirstChunkID(docID)
		if firstChunkID == "" {
			continue
		}
		seen[docID] = struct{}{}
		ids = append(ids, firstChunkID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	records, err := f.store.Get(ctx, collection, ids)
	if err != nil {
		// 取消/超时必须上抛，不能把 ctx 失效降级成正常结果
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if errors.Is(err, vectordb.ErrNotFound) {
			return nil, err
		}
		// 其他辅助查询失败时降级为不过滤，避免检索整体失败
		logger.Warn(ctx, "ai/retrieval 文档过滤器读取版本失败，降级为不过滤旧块", "collection", collection, "error", err)
		return nil, nil
	}
	states := make(map[string]documentState, len(records))
	for _, record := range records {
		docID := strings.TrimSpace(record.Metadata[f.cfg.DocumentKey])
		if docID == "" {
			continue
		}
		total, err := strconv.Atoi(strings.TrimSpace(record.Metadata[f.cfg.CountKey]))
		if err != nil || total <= 0 {
			continue
		}
		states[docID] = documentState{
			total:   total,
			version: strings.TrimSpace(record.Metadata[f.cfg.VersionKey]),
		}
	}
	return states, nil
}
