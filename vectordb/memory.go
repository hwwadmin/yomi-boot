// Package vectordb 进程内内存实现。
//
// @author zwei
package vectordb

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

var _ Store = (*memoryStore)(nil)

// memRow 内存中的一条记录
type memRow struct {
	id       string
	vector   []float32
	metadata map[string]string
}

// memoryStore 进程内向量库
type memoryStore struct {
	mu         sync.RWMutex
	closed     bool
	dim        int
	metric     Metric
	timeout    time.Duration
	maxRecords int
	colls      map[string]map[string]memRow
}

// newMemoryStore 创建内存 Store
func newMemoryStore(cfg Config) *memoryStore {
	return &memoryStore{
		dim:        cfg.Dimension,
		metric:     metricOrDefault(cfg.Metric),
		timeout:    timeoutOrDefault(cfg.Timeout),
		maxRecords: maxRecordsOrDefault(cfg.Memory.MaxRecords),
		colls:      make(map[string]map[string]memRow),
	}
}

// CreateCollection 创建空集合；已存在返回 ErrAlreadyExists
func (s *memoryStore) CreateCollection(ctx context.Context, name string) error {
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
	if _, ok := s.colls[n]; ok {
		return errors.Sentinel(ErrAlreadyExists)
	}
	s.colls[n] = make(map[string]memRow)
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

// ReplaceCollection 完整覆盖集合内容；不存在则创建；空切片保留空集合
func (s *memoryStore) ReplaceCollection(ctx context.Context, name string, records []Record) error {
	_, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(name)
	if err != nil {
		return err
	}
	if len(records) > 0 {
		if err := validateRecords(records, s.dim); err != nil {
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
		oldCount = len(old)
	}
	if s.totalLocked()-oldCount+len(next) > s.maxRecords {
		return errors.Sentinel(ErrInvalidArgument)
	}
	s.colls[n] = next
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
	if err := validateRecords(records, s.dim); err != nil {
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
	added := 0
	for id := range newIDs {
		if _, exists := coll[id]; !exists {
			added++
		}
	}
	if s.totalLocked()+added > s.maxRecords {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for _, row := range rows {
		coll[row.id] = row
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
		row, ok := coll[id]
		if !ok {
			continue
		}
		out = append(out, row.toRecord())
	}
	return out, nil
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
		delete(coll, strings.TrimSpace(raw))
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
	for id, row := range coll {
		if matchFilter(row.metadata, filter) {
			delete(coll, id)
		}
	}
	return nil
}

// Search 暴力扫描 TopK
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
	if err := validateSearchQuery(q, s.dim); err != nil {
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
	type scored struct {
		row   memRow
		score float32
	}
	cands := make([]scored, 0, len(coll))
	nchk := 0
	for _, row := range coll {
		nchk++
		if nchk&63 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, errors.Wrap(err, "vectordb")
			}
		}
		if !matchFilter(row.metadata, q.Filter) {
			continue
		}
		cands = append(cands, scored{row: row, score: similarityScore(s.metric, q.Vector, row.vector)})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score == cands[j].score {
			return cands[i].row.id < cands[j].row.id
		}
		return cands[i].score > cands[j].score
	})
	if q.TopK < len(cands) {
		cands = cands[:q.TopK]
	}
	out := make([]Hit, 0, len(cands))
	for _, c := range cands {
		out = append(out, Hit{ID: c.row.id, Score: c.score, Metadata: copyMetadata(c.row.metadata)})
	}
	return out, nil
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
		n += len(c)
	}
	return n
}

// toMemRow 将 Record 转为内部行
func toMemRow(r Record) memRow {
	id := strings.TrimSpace(r.ID)
	return memRow{
		id:       id,
		vector:   copyVector(r.Vector),
		metadata: copyMetadata(r.Metadata),
	}
}

// toRecord 转为导出 Record
func (r memRow) toRecord() Record {
	return Record{
		ID:       r.id,
		Vector:   copyVector(r.vector),
		Metadata: copyMetadata(r.metadata),
	}
}
