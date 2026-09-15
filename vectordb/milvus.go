// Package vectordb Milvus 实现。
//
// @author zwei
package vectordb

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"gitee.com/hq_project/yomi-boot/errors"
)

const (
	fieldID       = "id"
	fieldVector   = "vector"
	fieldMetadata = "metadata"
)

var _ Store = (*milvusStore)(nil)

// milvusStore 基于官方 client/v2 的 Store 实现
type milvusStore struct {
	mu      sync.Mutex
	closed  bool
	cli     *milvusclient.Client
	dim     int
	metric  Metric
	timeout time.Duration
}

// newMilvusStore 建连并构造 milvus Store
func newMilvusStore(cfg Config) (*milvusStore, error) {
	ctx, cancel := withTimeout(context.Background(), timeoutOrDefault(cfg.Timeout))
	defer cancel()
	cli, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address:       cfg.Milvus.Address,
		Username:      cfg.Milvus.Username,
		Password:      cfg.Milvus.Password,
		DBName:        cfg.Milvus.DBName,
		APIKey:        cfg.Milvus.APIKey,
		EnableTLSAuth: cfg.Milvus.EnableTLS,
	})
	if err != nil {
		return nil, wrapSentinel(ErrUnavailable, err)
	}
	return &milvusStore{
		cli:     cli,
		dim:     cfg.Dimension,
		metric:  metricOrDefault(cfg.Metric),
		timeout: timeoutOrDefault(cfg.Timeout),
	}, nil
}

// CreateCollection 创建空集合并建立索引、load；失败时 Drop 刚建集合
func (s *milvusStore) CreateCollection(ctx context.Context, name string) error {
	ctx, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(name)
	if err != nil {
		return err
	}
	has, err := s.has(ctx, n)
	if err != nil {
		return err
	}
	if has {
		return errors.Sentinel(ErrAlreadyExists)
	}
	return s.createReady(ctx, n)
}

// DropCollection 删除集合；不存在时幂等成功
func (s *milvusStore) DropCollection(ctx context.Context, name string) error {
	ctx, cancel, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	n, err := normalizeCollectionName(name)
	if err != nil {
		return err
	}
	has, err := s.has(ctx, n)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	if err := s.cli.DropCollection(ctx, milvusclient.NewDropCollectionOption(n)); err != nil {
		return mapMilvusErr(err)
	}
	return nil
}

// ReplaceCollection 完整覆盖集合内容；已存在时先写新快照再删旧点
func (s *milvusStore) ReplaceCollection(ctx context.Context, name string, records []Record) error {
	ctx, cancel, err := s.begin(ctx)
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
	has, err := s.has(ctx, n)
	if err != nil {
		return err
	}
	if !has {
		if err := s.createReady(ctx, n); err != nil && !errors.Is(err, ErrAlreadyExists) {
			return err
		}
	} else if err := s.ensureReady(ctx, n); err != nil {
		return err
	}
	keep := make(map[string]struct{}, len(records))
	if len(records) > 0 {
		for i := range records {
			keep[strings.TrimSpace(records[i].ID)] = struct{}{}
		}
		if err := s.upsertRecords(ctx, n, records); err != nil {
			return err
		}
		if err := s.flush(ctx, n); err != nil {
			return err
		}
	}
	stale, err := s.idsNotIn(ctx, n, keep)
	if err != nil {
		return err
	}
	if err := s.deleteIDs(ctx, n, stale); err != nil {
		return err
	}
	return s.flush(ctx, n)
}

// HasCollection 查询集合是否存在
func (s *milvusStore) HasCollection(ctx context.Context, name string) (bool, error) {
	ctx, cancel, err := s.begin(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	n, err := normalizeCollectionName(name)
	if err != nil {
		return false, err
	}
	return s.has(ctx, n)
}

// Upsert 按 ID 覆盖写入
func (s *milvusStore) Upsert(ctx context.Context, collection string, records []Record) error {
	ctx, cancel, err := s.begin(ctx)
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
	has, err := s.has(ctx, n)
	if err != nil {
		return err
	}
	if !has {
		return errors.Sentinel(ErrNotFound)
	}
	if err := s.ensureReady(ctx, n); err != nil {
		return err
	}
	if err := s.upsertRecords(ctx, n, records); err != nil {
		return err
	}
	return s.flush(ctx, n)
}

// Get 按 ID 取回命中记录
func (s *milvusStore) Get(ctx context.Context, collection string, ids []string) ([]Record, error) {
	ctx, cancel, err := s.begin(ctx)
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
	has, err := s.has(ctx, n)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, errors.Sentinel(ErrNotFound)
	}
	if err := s.ensureReady(ctx, n); err != nil {
		return nil, err
	}
	trimmed := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		trimmed = append(trimmed, id)
	}
	rs, err := s.cli.Query(ctx, milvusclient.NewQueryOption(n).
		WithIDs(column.NewColumnVarChar(fieldID, trimmed)).
		WithOutputFields(fieldID, fieldVector, fieldMetadata).
		WithConsistencyLevel(entity.ClStrong))
	if err != nil {
		return nil, mapMilvusErr(err)
	}
	return resultSetToRecords(&rs)
}

// Delete 按 ID 删除
func (s *milvusStore) Delete(ctx context.Context, collection string, ids []string) error {
	ctx, cancel, err := s.begin(ctx)
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
	has, err := s.has(ctx, n)
	if err != nil {
		return err
	}
	if !has {
		return errors.Sentinel(ErrNotFound)
	}
	if err := s.ensureReady(ctx, n); err != nil {
		return err
	}
	trimmed := make([]string, len(ids))
	for i, raw := range ids {
		trimmed[i] = strings.TrimSpace(raw)
	}
	if err := s.deleteIDs(ctx, n, trimmed); err != nil {
		return err
	}
	return s.flush(ctx, n)
}

// DeleteByFilter 按元数据等值 AND 删除
func (s *milvusStore) DeleteByFilter(ctx context.Context, collection string, filter map[string]string) error {
	ctx, cancel, err := s.begin(ctx)
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
	has, err := s.has(ctx, n)
	if err != nil {
		return err
	}
	if !has {
		return errors.Sentinel(ErrNotFound)
	}
	if err := s.ensureReady(ctx, n); err != nil {
		return err
	}
	expr, err := metadataFilterExpr(filter)
	if err != nil {
		return err
	}
	if _, err := s.cli.Delete(ctx, milvusclient.NewDeleteOption(n).WithExpr(expr)); err != nil {
		return mapMilvusErr(err)
	}
	return s.flush(ctx, n)
}

// Search 向量检索
func (s *milvusStore) Search(ctx context.Context, collection string, q SearchQuery) ([]Hit, error) {
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
	has, err := s.has(ctx, n)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, errors.Sentinel(ErrNotFound)
	}
	if err := s.ensureReady(ctx, n); err != nil {
		return nil, err
	}
	opt := milvusclient.NewSearchOption(n, q.TopK, []entity.Vector{entity.FloatVector(q.Vector)}).
		WithANNSField(fieldVector).
		WithOutputFields(fieldID, fieldMetadata).
		WithConsistencyLevel(entity.ClStrong)
	if len(q.Filter) > 0 {
		expr, err := metadataFilterExpr(q.Filter)
		if err != nil {
			return nil, err
		}
		opt = opt.WithFilter(expr)
	}
	sets, err := s.cli.Search(ctx, opt)
	if err != nil {
		return nil, mapMilvusErr(err)
	}
	if len(sets) == 0 {
		return []Hit{}, nil
	}
	return resultSetToHits(&sets[0], s.metric)
}

// Close 关闭 gRPC 客户端
func (s *milvusStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.cli == nil {
		return nil
	}
	return mapEngineError(s.cli.Close(context.Background()))
}

// begin 检查未关闭并套超时
func (s *milvusStore) begin(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if err := s.errIfClosed(); err != nil {
		return nil, nil, err
	}
	ctx, cancel := withTimeout(ctx, s.timeout)
	return ctx, cancel, nil
}

// errIfClosed 已关闭则返回 ErrClosed
func (s *milvusStore) errIfClosed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.Sentinel(ErrClosed)
	}
	return nil
}

// has 调用 SDK HasCollection
func (s *milvusStore) has(ctx context.Context, name string) (bool, error) {
	ok, err := s.cli.HasCollection(ctx, milvusclient.NewHasCollectionOption(name))
	if err != nil {
		return false, mapMilvusErr(err)
	}
	return ok, nil
}

// createReady 创建空集合并建立索引、load；索引或 load 失败则 Drop，避免半就绪
func (s *milvusStore) createReady(ctx context.Context, name string) error {
	schema := entity.NewSchema().
		WithField(entity.NewField().WithName(fieldID).WithDataType(entity.FieldTypeVarChar).WithMaxLength(int64(maxIDLen)).WithIsPrimaryKey(true)).
		WithField(entity.NewField().WithName(fieldVector).WithDataType(entity.FieldTypeFloatVector).WithDim(int64(s.dim))).
		WithField(entity.NewField().WithName(fieldMetadata).WithDataType(entity.FieldTypeJSON))
	if err := s.cli.CreateCollection(ctx, milvusclient.NewCreateCollectionOption(name, schema)); err != nil {
		return mapMilvusErr(err)
	}
	if err := s.createIndex(ctx, name); err != nil {
		s.dropQuiet(ctx, name)
		return err
	}
	if err := s.load(ctx, name); err != nil {
		s.dropQuiet(ctx, name)
		return err
	}
	return nil
}

// ensureReady 补齐缺失索引并 load；已 load 则跳过
func (s *milvusStore) ensureReady(ctx context.Context, name string) error {
	state, err := s.cli.GetLoadState(ctx, milvusclient.NewGetLoadStateOption(name))
	if err != nil {
		return mapMilvusErr(err)
	}
	if state.State == entity.LoadStateLoaded {
		return nil
	}
	if err := s.createIndex(ctx, name); err != nil {
		return err
	}
	return s.load(ctx, name)
}

// createIndex 为向量字段建 HNSW；已存在视为成功
func (s *milvusStore) createIndex(ctx context.Context, name string) error {
	idx := index.NewHNSWIndex(toEntityMetric(s.metric), 16, 64)
	task, err := s.cli.CreateIndex(ctx, milvusclient.NewCreateIndexOption(name, fieldVector, idx))
	if err != nil {
		mapped := mapMilvusErr(err)
		if errors.Is(mapped, ErrAlreadyExists) {
			return nil
		}
		return mapped
	}
	if err := task.Await(ctx); err != nil {
		return mapMilvusErr(err)
	}
	return nil
}

// dropQuiet 补偿删除；忽略错误以免掩盖主因
func (s *milvusStore) dropQuiet(ctx context.Context, name string) {
	_ = s.cli.DropCollection(ctx, milvusclient.NewDropCollectionOption(name))
}

// load 加载集合并等待完成
func (s *milvusStore) load(ctx context.Context, name string) error {
	task, err := s.cli.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(name))
	if err != nil {
		return mapMilvusErr(err)
	}
	if err := task.Await(ctx); err != nil {
		return mapMilvusErr(err)
	}
	return nil
}

// flush 刷盘并等待完成
func (s *milvusStore) flush(ctx context.Context, name string) error {
	task, err := s.cli.Flush(ctx, milvusclient.NewFlushOption(name))
	if err != nil {
		return mapMilvusErr(err)
	}
	if err := task.Await(ctx); err != nil {
		return mapMilvusErr(err)
	}
	return nil
}

// upsertRecords 分批 Upsert
func (s *milvusStore) upsertRecords(ctx context.Context, name string, records []Record) error {
	for start := 0; start < len(records); start += upsertBatchSize {
		end := start + upsertBatchSize
		if end > len(records) {
			end = len(records)
		}
		batch := records[start:end]
		ids := make([]string, len(batch))
		vecs := make([][]float32, len(batch))
		metas := make([][]byte, len(batch))
		for i := range batch {
			ids[i] = strings.TrimSpace(batch[i].ID)
			vecs[i] = batch[i].Vector
			raw, err := marshalMetadata(batch[i].Metadata)
			if err != nil {
				return errors.Sentinel(ErrInvalidArgument)
			}
			metas[i] = raw
		}
		_, err := s.cli.Upsert(ctx, milvusclient.NewColumnBasedInsertOption(name).
			WithVarcharColumn(fieldID, ids).
			WithFloatVectorColumn(fieldVector, s.dim, vecs).
			WithColumns(column.NewColumnJSONBytes(fieldMetadata, metas)))
		if err != nil {
			return mapMilvusErr(err)
		}
	}
	return nil
}

// idsNotIn 列出集合中不在 keep 内的主键
func (s *milvusStore) idsNotIn(ctx context.Context, name string, keep map[string]struct{}) ([]string, error) {
	iter, err := s.cli.QueryIterator(ctx, milvusclient.NewQueryIteratorOption(name).
		WithOutputFields(fieldID).
		WithBatchSize(1000).
		WithConsistencyLevel(entity.ClStrong))
	if err != nil {
		return nil, mapMilvusErr(err)
	}
	var stale []string
	for {
		rs, err := iter.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, mapMilvusErr(err)
		}
		ids, err := idsFromResultSet(&rs)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, ok := keep[id]; !ok {
				stale = append(stale, id)
			}
		}
	}
	return stale, nil
}

// deleteIDs 按主键分批删除；空切片成功
func (s *milvusStore) deleteIDs(ctx context.Context, name string, ids []string) error {
	for start := 0; start < len(ids); start += upsertBatchSize {
		end := start + upsertBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		if _, err := s.cli.Delete(ctx, milvusclient.NewDeleteOption(name).WithStringIDs(fieldID, ids[start:end])); err != nil {
			return mapMilvusErr(err)
		}
	}
	return nil
}

// toEntityMetric 将本包 Metric 转为 SDK 度量
func toEntityMetric(m Metric) entity.MetricType {
	switch metricOrDefault(m) {
	case MetricL2:
		return entity.L2
	case MetricIP:
		return entity.IP
	default:
		return entity.COSINE
	}
}

// metadataFilterExpr 生成 JSON 等值 AND 表达式
func metadataFilterExpr(filter map[string]string) (string, error) {
	if err := validateFilter(filter, true); err != nil {
		return "", err
	}
	parts := make([]string, 0, len(filter))
	for k, v := range filter {
		key := strings.TrimSpace(k)
		parts = append(parts, fieldMetadata+`["`+key+`"] == `+quoteMilvusString(v))
	}
	return strings.Join(parts, " && "), nil
}

// quoteMilvusString 将字符串写成 Milvus 字面量
func quoteMilvusString(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(s) + `"`
}

// marshalMetadata 将元数据编码为 JSON 对象
func marshalMetadata(md map[string]string) ([]byte, error) {
	copied := copyMetadata(md)
	if copied == nil {
		copied = map[string]string{}
	}
	return json.Marshal(copied)
}

// idsFromResultSet 从结果集读取主键列
func idsFromResultSet(rs *milvusclient.ResultSet) ([]string, error) {
	if rs == nil || rs.Len() == 0 {
		return nil, nil
	}
	idCol := rs.IDs
	if idCol == nil {
		idCol = rs.GetColumn(fieldID)
	}
	if idCol == nil {
		return nil, errors.Sentinel(ErrUnavailable)
	}
	out := make([]string, 0, rs.Len())
	for i := 0; i < rs.Len(); i++ {
		id, err := idCol.GetAsString(i)
		if err != nil {
			return nil, wrapSentinel(ErrUnavailable, err)
		}
		out = append(out, id)
	}
	return out, nil
}

// resultSetToHits 将检索结果转为 Hit
func resultSetToHits(rs *milvusclient.ResultSet, metric Metric) ([]Hit, error) {
	if rs == nil || rs.Len() == 0 {
		return []Hit{}, nil
	}
	ids, err := idsFromResultSet(rs)
	if err != nil {
		return nil, err
	}
	metaCol := rs.GetColumn(fieldMetadata)
	out := make([]Hit, 0, len(ids))
	for i, id := range ids {
		score := float32(0)
		if i < len(rs.Scores) {
			score = engineScoreToHit(metric, rs.Scores[i])
		}
		md, err := metadataAt(metaCol, i)
		if err != nil {
			return nil, err
		}
		out = append(out, Hit{ID: id, Score: score, Metadata: md})
	}
	return out, nil
}

// resultSetToRecords 将 Query 结果转为 Record
func resultSetToRecords(rs *milvusclient.ResultSet) ([]Record, error) {
	if rs == nil || rs.Len() == 0 {
		return []Record{}, nil
	}
	ids, err := idsFromResultSet(rs)
	if err != nil {
		return nil, err
	}
	vecCol := rs.GetColumn(fieldVector)
	metaCol := rs.GetColumn(fieldMetadata)
	out := make([]Record, 0, len(ids))
	for i, id := range ids {
		vec, err := vectorAt(vecCol, i)
		if err != nil {
			return nil, err
		}
		md, err := metadataAt(metaCol, i)
		if err != nil {
			return nil, err
		}
		out = append(out, Record{ID: id, Vector: vec, Metadata: md})
	}
	return out, nil
}

// vectorAt 读取第 i 条向量
func vectorAt(col column.Column, i int) ([]float32, error) {
	if col == nil {
		return nil, errors.Sentinel(ErrUnavailable)
	}
	raw, err := col.Get(i)
	if err != nil {
		return nil, wrapSentinel(ErrUnavailable, err)
	}
	switch v := raw.(type) {
	case []float32:
		return copyVector(v), nil
	case entity.FloatVector:
		return copyVector([]float32(v)), nil
	default:
		return nil, errors.Sentinel(ErrUnavailable)
	}
}

// metadataAt 读取第 i 条 JSON 元数据
func metadataAt(col column.Column, i int) (map[string]string, error) {
	if col == nil {
		return nil, nil
	}
	raw, err := col.Get(i)
	if err != nil {
		return nil, wrapSentinel(ErrUnavailable, err)
	}
	var bs []byte
	switch v := raw.(type) {
	case []byte:
		bs = v
	case string:
		bs = []byte(v)
	default:
		return nil, nil
	}
	if len(bs) == 0 || string(bs) == "null" {
		return nil, nil
	}
	var md map[string]string
	if err := json.Unmarshal(bs, &md); err != nil {
		return nil, wrapSentinel(ErrUnavailable, err)
	}
	return copyMetadata(md), nil
}

// mapMilvusErr 将 SDK 错误映射为本包哨兵
func mapMilvusErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "already exist"):
		return wrapSentinel(ErrAlreadyExists, err)
	case strings.Contains(msg, "not found"), strings.Contains(msg, "does not exist"), strings.Contains(msg, "doesn't exist"):
		return wrapSentinel(ErrNotFound, err)
	default:
		return mapEngineError(err)
	}
}
