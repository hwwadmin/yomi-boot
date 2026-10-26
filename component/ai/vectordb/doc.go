// Package vectordb 提供统一向量库门面，支持 milvus 与 memory。
//
// 配置只在 New / MustNew 传入；本包不读 YAML、不进插件链、不注册配置根段。
// 一个 Store 锁定 Metric（空则 COSINE）与 milvus IndexType（空则 HNSW）；向量维度在创建集合时传入并锁定在集合上，不同集合可不同。
// 集合名每次调用传入，用于动态新增、删除、覆盖索引。已存在集合不会因改 IndexType 而重建索引。
// 业务禁止直引 github.com/milvus-io/milvus/client/v2。
//
//	store, err := vectordb.New(vectordb.Config{
//	    Provider: vectordb.ProviderMemory,
//	})
//	err = store.CreateCollection(ctx, "kb_sales", 1024)
//
// CreateCollection 接收集合向量维度，已存在返回 ErrAlreadyExists。HasCollection 不存在返回 false, nil。
// milvus 建集时索引或 load 失败会删掉刚建的集合，避免半就绪；读写前会对未 load 的集合补索引并 load。
// ReplaceCollection 用完整快照覆盖，集合不存在时按传入维度创建；过程允许脏读；milvus 先写新点再删旧点，失败时旧数据仍在（可能夹杂已写入的新点）；空切片保留空集合。
// ListIDs 按元数据等值 AND 列出主键，只返回 ID；用于按业务键枚举/清理，不读向量。
// EnsureCollection 确保集合存在；已存在且维度不一致返回 ErrInvalidArgument。
// ReplaceByFilter 按过滤键覆盖写入：先 Upsert records，再删除过滤命中但不在 records 中的
// 旧主键；records 为空时只清理；同一过滤键的替换在本实例内串行。
// Close 由业务在进程退出时调用，plugin.PluginSlot 不会自动关闭。
//
// # 检索模式
//
// Search 的 SearchQuery.Text 决定检索模式：
//
//   - Text 为空：纯向量检索，Score 越大越相似（L2 已映射）；不同 Metric 的 Score 不可横比
//   - Text 非空：向量 + BM25 混合检索，两路结果以 RRF（k=60）融合，
//     Score 是融合排名分（量级约 0 ~ 2/61，两路都命中的头部最大），与纯向量 Score 不可横比
//
// BM25 词法命中针对 Record.Text 原文；Text 为空的记录只参与向量一路。
// 混合检索中两路各自最多取 TopK 条参与融合，Filter 对两路同时生效。
//
// milvus 侧混合检索要求 Milvus 2.5+（BM25 全文检索与 jieba 分词器）；
// 集合 schema 自带 text / sparse 字段与 BM25 Function，旧版本创建的集合不含这些字段，
// 写入与混合检索会得到引擎错误，需 DropCollection 后重建。
// memory 侧自带 BM25（ASCII 词 + CJK 单字与二元词组切词，参数 k1=1.2、b=0.75，语料统计按过滤后候选集计算），
// 排序与 milvus 的 jieba 分词实现存在合理差异，契约只保证"词法命中提升排名"一致。
//
// @author zwei
package vectordb
