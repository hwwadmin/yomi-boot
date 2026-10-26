// 进程内内存实现。
//
// @author zwei

package vectordb

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

var _ Store = (*memoryStore)(nil)

// memRow 内存中的一条记录
type memRow struct {
	id       string
	vector   []float32
	text     string
	metadata map[string]string
}

// memCollection 内存中的一个集合
type memCollection struct {
	dim  int
	rows map[string]memRow
}

// memoryStore 进程内向量库
type memoryStore struct {
	mu           sync.RWMutex
	closed       bool
	metric       Metric
	timeout      time.Duration
	maxRecords   int
	colls        map[string]*memCollection
	replaceLocks replaceLock
}

// newMemoryStore 创建内存 Store
func newMemoryStore(cfg Config) *memoryStore {
	return &memoryStore{
		metric:     metricOrDefault(cfg.Metric),
		timeout:    timeoutOrDefault(cfg.Timeout),
		maxRecords: maxRecordsOrDefault(cfg.Memory.MaxRecords),
		colls:      make(map[string]*memCollection),
	}
}

// CreateCollection 创建空集合；已存在返回 ErrAlreadyExists
func (s *memoryStore) CreateCollection(ctx context.Context, name string, dimension int) error {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(name)
	if err != nil {
		return err
	}
	if err := validateDimension(dimension); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.Sentinel(ErrClosed)
	}
	if _, ok := s.colls[n]; ok {
		return errors.Sentinel(ErrAlreadyExists)
	}
	s.colls[n] = &memCollection{dim: dimension, rows: make(map[string]memRow)}
	return nil
}

// DropCollection 删除集合；不存在时幂等成功
func (s *memoryStore) DropCollection(ctx context.Context, name string) error {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.Sentinel(ErrClosed)
	}
	delete(s.colls, n)
	return nil
}

// ReplaceCollection 完整覆盖集合内容；不存在则按 dimension 创建；已存在且维度不一致返回 ErrInvalidArgument
func (s *memoryStore) ReplaceCollection(ctx context.Context, name string, dimension int, records []Record) error {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(name)
	if err != nil {
		return err
	}
	if err := validateDimension(dimension); err != nil {
		return err
	}
	if len(records) > 0 {
		if err := validateRecords(records, dimension); err != nil {
			return err
		}
	}
	next := make(map[string]memRow, len(records))
	for i := range records {
		row := toMemRow(records[i])
		next[row.id] = row
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.Sentinel(ErrClosed)
	}
	oldCount := 0
	if old, ok := s.colls[n]; ok {
		if old.dim != dimension {
			return errors.Sentinel(ErrInvalidArgument)
		}
		oldCount = len(old.rows)
	}
	if s.totalLocked()-oldCount+len(next) > s.maxRecords {
		return errors.Sentinel(ErrInvalidArgument)
	}
	s.colls[n] = &memCollection{dim: dimension, rows: next}
	return nil
}

// HasCollection 查询集合是否存在
func (s *memoryStore) HasCollection(ctx context.Context, name string) (bool, error) {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	n, err := normalizeCollectionName(name)
	if err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false, errors.Sentinel(ErrClosed)
	}
	_, ok := s.colls[n]
	return ok, nil
}

// EnsureCollection 确保集合存在；已存在且维度不一致返回 ErrInvalidArgument
func (s *memoryStore) EnsureCollection(ctx context.Context, name string, dimension int) error {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(name)
	if err != nil {
		return err
	}
	if err := validateDimension(dimension); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.Sentinel(ErrClosed)
	}
	coll, ok := s.colls[n]
	if !ok {
		s.colls[n] = &memCollection{dim: dimension, rows: make(map[string]memRow)}
		return nil
	}
	if coll.dim != dimension {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return nil
}

// Upsert 按 ID 覆盖写入
func (s *memoryStore) Upsert(ctx context.Context, collection string, records []Record) error {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(collection)
	if err != nil {
		return err
	}
	rows := make([]memRow, 0, len(records))
	newIDs := make(map[string]struct{}, len(records))
	for i := range records {
		row := toMemRow(records[i])
		rows = append(rows, row)
		newIDs[row.id] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.Sentinel(ErrClosed)
	}
	coll, ok := s.colls[n]
	if !ok {
		return errors.Sentinel(ErrNotFound)
	}
	if err := validateRecords(records, coll.dim); err != nil {
		return err
	}
	added := 0
	for id := range newIDs {
		if _, exists := coll.rows[id]; !exists {
			added++
		}
	}
	if s.totalLocked()+added > s.maxRecords {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for _, row := range rows {
		coll.rows[row.id] = row
	}
	return nil
}

// Get 按 ID 取回命中记录
func (s *memoryStore) Get(ctx context.Context, collection string, ids []string) ([]Record, error) {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	n, err := normalizeCollectionName(collection)
	if err != nil {
		return nil, err
	}
	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.Sentinel(ErrClosed)
	}
	coll, ok := s.colls[n]
	if !ok {
		return nil, errors.Sentinel(ErrNotFound)
	}
	out := make([]Record, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		row, ok := coll.rows[id]
		if !ok {
			continue
		}
		out = append(out, row.toRecord())
	}
	return out, nil
}

// ListIDs 按元数据等值 AND 列出主键；返回结果按 ID 升序
func (s *memoryStore) ListIDs(ctx context.Context, collection string, filter map[string]string) ([]string, error) {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	n, err := normalizeCollectionName(collection)
	if err != nil {
		return nil, err
	}
	if err := validateFilter(filter, true); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.Sentinel(ErrClosed)
	}
	coll, ok := s.colls[n]
	if !ok {
		return nil, errors.Sentinel(ErrNotFound)
	}
	ids := make([]string, 0, len(coll.rows))
	for id, row := range coll.rows {
		if matchFilter(row.metadata, filter) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// Delete 按 ID 删除
func (s *memoryStore) Delete(ctx context.Context, collection string, ids []string) error {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(collection)
	if err != nil {
		return err
	}
	if err := validateIDs(ids); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.Sentinel(ErrClosed)
	}
	coll, ok := s.colls[n]
	if !ok {
		return errors.Sentinel(ErrNotFound)
	}
	for _, raw := range ids {
		delete(coll.rows, strings.TrimSpace(raw))
	}
	return nil
}

// DeleteByFilter 按元数据等值 AND 删除
func (s *memoryStore) DeleteByFilter(ctx context.Context, collection string, filter map[string]string) error {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(collection)
	if err != nil {
		return err
	}
	if err := validateFilter(filter, true); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.Sentinel(ErrClosed)
	}
	coll, ok := s.colls[n]
	if !ok {
		return errors.Sentinel(ErrNotFound)
	}
	for id, row := range coll.rows {
		if matchFilter(row.metadata, filter) {
			delete(coll.rows, id)
		}
	}
	return nil
}

// ReplaceByFilter 按过滤键覆盖写入并清理旧记录
func (s *memoryStore) ReplaceByFilter(ctx context.Context, collection string, filter map[string]string, records []Record) error {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(collection)
	if err != nil {
		return err
	}
	if err := validateFilter(filter, true); err != nil {
		return err
	}
	unlock := s.replaceLocks.lock(n, filter)
	defer unlock()
	if len(records) > 0 {
		if err := s.Upsert(ctx, n, records); err != nil {
			return err
		}
	}
	ids, err := s.ListIDs(ctx, n, filter)
	if err != nil {
		return err
	}
	stale := staleIDs(ids, records)
	if len(stale) == 0 {
		return nil
	}
	return s.Delete(ctx, n, stale)
}

// Search 暴力扫描 TopK；Text 非空时走向量 + BM25 混合并以 RRF 融合
func (s *memoryStore) Search(ctx context.Context, collection string, q SearchQuery) ([]Hit, error) {
	ctx, cancel, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	n, err := normalizeCollectionName(collection)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.Sentinel(ErrClosed)
	}
	coll, ok := s.colls[n]
	if !ok {
		return nil, errors.Sentinel(ErrNotFound)
	}
	if err := validateSearchQuery(q, coll.dim); err != nil {
		return nil, err
	}
	rows := make([]memRow, 0, len(coll.rows))
	nchk := 0
	for _, row := range coll.rows {
		nchk++
		if nchk&63 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, errors.Wrap(err, "vectordb")
			}
		}
		if !matchFilter(row.metadata, q.Filter) {
			continue
		}
		rows = append(rows, row)
	}
	var out []Hit
	if q.Text == "" {
		out = s.denseHits(rows, q)
	} else {
		out = s.hybridHits(rows, q)
	}
	return out, nil
}

// denseHits 纯向量排序并截断 TopK
func (s *memoryStore) denseHits(rows []memRow, q SearchQuery) []Hit {
	dense := make([]rankedID, 0, len(rows))
	for _, row := range rows {
		dense = append(dense, rankedID{id: row.id, score: float64(similarityScore(s.metric, q.Vector, row.vector))})
	}
	rankDescending(dense)
	if q.TopK < len(dense) {
		dense = dense[:q.TopK]
	}
	return hitsByID(rows, dense)
}

// hybridHits 向量与 BM25 两路排名后 RRF 融合；BM25 路只保留有词法命中的条目
func (s *memoryStore) hybridHits(rows []memRow, q SearchQuery) []Hit {
	dense := make([]rankedID, 0, len(rows))
	for _, row := range rows {
		dense = append(dense, rankedID{id: row.id, score: float64(similarityScore(s.metric, q.Vector, row.vector))})
	}
	rankDescending(dense)
	if q.TopK < len(dense) {
		dense = dense[:q.TopK]
	}
	texts := make([]string, len(rows))
	for i, row := range rows {
		texts[i] = row.text
	}
	scorer := newBM25Scorer(texts)
	queryTokens := tokenizeText(q.Text)
	lexical := make([]rankedID, 0, len(rows))
	for i, row := range rows {
		if score := scorer.score(i, queryTokens); score > 0 {
			lexical = append(lexical, rankedID{id: row.id, score: score})
		}
	}
	rankDescending(lexical)
	if q.TopK < len(lexical) {
		lexical = lexical[:q.TopK]
	}
	fused := rrfFuse([][]rankedID{dense, lexical})
	if q.TopK < len(fused) {
		fused = fused[:q.TopK]
	}
	return hitsByID(rows, fused)
}

// hitsByID 按排名条目构造命中；score 为各排名条目的分值
func hitsByID(rows []memRow, ranked []rankedID) []Hit {
	byID := make(map[string]memRow, len(rows))
	for _, row := range rows {
		byID[row.id] = row
	}
	out := make([]Hit, 0, len(ranked))
	for _, item := range ranked {
		row, ok := byID[item.id]
		if !ok {
			continue
		}
		out = append(out, Hit{ID: item.id, Score: float32(item.score), Metadata: copyMetadata(row.metadata)})
	}
	return out
}

// Close 关闭内存 Store
func (s *memoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.colls = nil
	return nil
}

// begin 检查未关闭、ctx 未取消，并套超时
func (s *memoryStore) begin(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, nil, errors.Wrap(err, "vectordb")
		}
	}
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return nil, nil, errors.Sentinel(ErrClosed)
	}
	ctx, cancel := withTimeout(ctx, s.timeout)
	return ctx, cancel, nil
}

// totalLocked 返回当前总条数；调用方须持锁
func (s *memoryStore) totalLocked() int {
	n := 0
	for _, c := range s.colls {
		n += len(c.rows)
	}
	return n
}

// toMemRow 将 Record 转为内部行
func toMemRow(r Record) memRow {
	id := strings.TrimSpace(r.ID)
	return memRow{
		id:       id,
		vector:   copyVector(r.Vector),
		text:     r.Text,
		metadata: copyMetadata(r.Metadata),
	}
}

// toRecord 转为导出 Record
func (r memRow) toRecord() Record {
	return Record{
		ID:       r.id,
		Vector:   copyVector(r.vector),
		Text:     r.text,
		Metadata: copyMetadata(r.metadata),
	}
}
