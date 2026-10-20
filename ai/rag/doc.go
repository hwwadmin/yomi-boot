// Package rag 提供基于 ai/embedding 与 vectordb 的检索增强数据能力。
//
// 覆盖切块、向量化、写入、检索与删除，不包含大模型问答。
// Config 只保留 Embedder 与 Store；集合名与其余运行期参数都按调用传入，
// 同一个 Engine 可并发服务多个集合（不同业务使用不同集合名即可）。
// 配置只在 New / MustNew 传入；本包不读 YAML、不进 bootstrap、不注册配置根段。
// 本包不持有 Embedder / Store 的关闭权，进程退出时由调用方自行关闭。
//
// # 基本用法
//
//	engine, err := rag.New(rag.Config{Embedder: emb, Store: store})
//	splitter, err := rag.NewSplitter(rag.SplitConfig{Strategy: rag.StrategyMarkdown})
//	_, err = engine.Index(ctx, "kb_sales", rag.IndexRequest{
//	    Documents: []rag.Document{{ID: "doc-1", Content: text, Source: "manual.md"}},
//	    Splitter:  splitter,
//	})
//	chunks, err := engine.Retrieve(ctx, "kb_sales", rag.RetrieveRequest{Query: "如何退款", TopK: 5})
//	err = engine.DeleteDocument(ctx, "kb_sales", "doc-1")
//
// # 多集合
//
// 集合名每次调用传入；Index 对不存在的集合自动创建，Retrieve 在集合不存在时返回空切片，
// DeleteDocument 在集合不存在时返回 nil。不同业务共用同一个 Engine 时，用不同集合名隔离数据即可；
// 所有集合共用同一 Store 的向量维度与 Metric。
//
// # 切块策略
//
// Splitter 是切块策略门面，内置四种策略：
//
//   - recursive（默认）：按段落、行、中英文句末、分句与空格逐级回退
//   - fixed：按固定字符窗口切分，可能截断词语
//   - sentence：按段落与中英文句末切分后装箱
//   - markdown：按 Markdown 标题分节，超长小节再递归切分
//
// 通过 IndexRequest.Splitter 按需指定；为空则使用内置默认 recursive（800 字 / 100 重叠）。
// 可用 NewSplitter / SplitText 直接构造使用。块长按 rune 计，单块不得超过 8192 字。
//
// # 运行期参数
//
//   - 集合名、IndexRequest.Splitter、RetrieveRequest.TopK/MinScore/Filter 都按调用传入
//   - RetrieveRequest.TopK 必填；MinScore 为 0 表示不设阈值
//   - Engine 不内置超时，使用调用方 ctx 控制
//   - 向量维度在首次向量化后锁定；Embedder 输出变化返回 ErrDimensionMismatch
//   - Store 的维度与 Metric 由 Store 构造配置决定，其下所有集合一致
//
// # 写入
//
//   - 块主键为 "文档ID#序号"；内容与切分信息写入保留元数据键，文档自定义元数据原样合并
//   - 同一文档重复 Index 会覆盖同序号块，并删除序号超出新块数的旧块；失败时可能留下部分新块，重试即可
//   - 旧块清理依据块上的 rag_chunk_count，仅对由本库写入的块生效
//   - 同一文档的 Index 需调用方串行；不同文档、不同集合可并发
//   - 保留元数据键见 MetadataKeyDocumentID 等常量，Document.Metadata 不得使用
//
// # 检索与删除
//
//   - Retrieve：集合不存在或为空时返回空切片与 nil，不视为错误；缺少 rag_content 的命中会被忽略
//   - Score 越大越相似；不同 Metric 的 Score 不可横比
//   - DeleteDocument：删除文档全部块；集合不存在时返回 nil
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument / ErrDimensionMismatch。
// embedding / vectordb 的错误原样透传，可用 errors.Is 判断其哨兵。
//
// Engine 构造后不可变，可并发调用；请勿复制 Engine 值。
//
// @author zwei
package rag
