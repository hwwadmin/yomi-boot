// Package vectordb 提供统一向量库门面，支持 milvus 与 memory。
//
// 配置只在 New / MustNew 传入；本包不读 YAML、不进 bootstrap、不注册配置根段。
// 一个 Store 锁定 Dimension 与 Metric（空则 COSINE）；milvus 还锁定 IndexType（空则 HNSW）。
// 集合名每次调用传入，用于动态新增、删除、覆盖索引。已存在集合不会因改 IndexType 而重建索引。
// 业务禁止直引 github.com/milvus-io/milvus/client/v2。
//
//	store, err := vectordb.New(vectordb.Config{
//	    Provider:  vectordb.ProviderMemory,
//	    Dimension: 1024,
//	})
//
// CreateCollection 已存在返回 ErrAlreadyExists。HasCollection 不存在返回 false, nil。
// milvus 建集时索引或 load 失败会删掉刚建的集合，避免半就绪；读写前会对未 load 的集合补索引并 load。
// ReplaceCollection 用完整快照覆盖；过程允许脏读；milvus 先写新点再删旧点，失败时旧数据仍在（可能夹杂已写入的新点）；空切片保留空集合。
// Search 的 Hit.Score 越大越相似（L2 已映射）；不同 Metric 的 Score 不可横比。
// Close 由业务在进程退出时调用，Infra.Cleanup 不会自动关闭。
//
// @author zwei
package vectordb
