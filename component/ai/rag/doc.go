// Package rag 提供两条 RAG 流程的编排：文档入库与文档检索增强。
//
// 两条流程由 5 个门面组件协作：切块（ai/chunking）、向量化（ai/embedding）、
// 检索（ai/retrieval）、精重排（ai/rerank）与提示词拼接（ai/generation），
// 构造时经 Config 全部注入；向量库（vectordb.Store）负责数据存取。
// 本包负责流程串联与文档域约定：块主键、保留元数据键、文档版本与旧块清理。
// 同一个 Engine 可并发服务多个集合；配置经 New / MustNew 传入，各依赖的关闭由调用方负责。
//
// # 基本用法
//
//	emb, err := embedding.New(embedding.Config{ /* ... */ })
//	store, err := vectordb.New(vectordb.Config{ /* ... */ })
//	chunker, err := chunking.New(chunking.Config{
//	    Strategy: chunking.StrategyMarkdown,
//	    Markdown: chunking.MarkdownConfig{ChunkSize: 800, MaxHeadingLevel: 3},
//	})
//	retriever, err := retrieval.New(retrieval.Config{Embedder: emb, Store: store})
//	engine := rag.MustNew(rag.Config{
//	    Chunker:   chunker,
//	    Embedder:  emb,
//	    Retriever: retriever,
//	    Reranker:  rerank.NewPassThrough(), // 未接入重排模型时的占位
//	    Generator: generation.NewBasic(),   // 未定制提示词模板时的占位
//	    Store:     store,
//	})
//	_, err = engine.Index(ctx, "kb_sales", rag.IndexRequest{
//	    Documents: []rag.Document{{ID: "doc-1", Content: text, Source: "manual.md"}},
//	})
//	result, err := engine.Retrieve(ctx, "kb_sales", rag.RetrieveRequest{
//	    RetrieveRequest: retrieval.RetrieveRequest{Query: "如何退款", TopK: 20},
//	    RerankTopN:      5,
//	})
//	// result.Prompt 直接作为大模型输入；result.Chunks 供引用标注
//
// # 两条流程
//
//	路径① 文档入库：Document → Chunker 切块 → Embedder 向量化 → Store 写入
//	路径② 文档检索增强：Query → Retriever 检索（含域过滤）→ Reranker 精重排
//	  → Generator 拼接提示词
//
// 装配要求：Retriever 与 Store 使用同一向量库；Embedder 与 Retriever 的查询
// 向量化使用同一模型与维度。
//
// # 多集合与文档删除
//
// 集合名每次调用传入：Index 按 IndexRequest.Dimensions 创建不存在的集合，Retrieve 在
// 集合不存在时返回空 Chunks 与由 Generator 拼接的 Prompt。不同业务用不同集合名隔离数据，
// 不同集合可有不同向量维度；Metric 由 Store 配置统一。集合创建/删除与文档删除直接使用
// vectordb.Store（保留键 MetadataKeyDocumentID 即 rag_doc_id）：
//
//	err = store.DeleteByFilter(ctx, "kb_sales", map[string]string{rag.MetadataKeyDocumentID: "doc-1"})
//
// # 切块
//
// Config.Chunker 是默认切块策略，内置 recursive / fixed / sentence / markdown 见
// component/ai/chunking；IndexRequest.Chunker 可按调用覆盖，例如 Markdown 文档与
// 纯文本使用不同策略。Index 用 chunking.Normalize 包装切块器，自定义实现的输出
// 由切块层校验（空白块丢弃、超长块返回 chunking.ErrInvalidArgument）。
// 块长按 rune 计，单块不超过 chunking.MaxChunkSize 字。
//
// # 运行期参数
//
//   - 集合名、IndexRequest 的 Dimensions/Chunker、RetrieveRequest 的 Strategy/TopK/
//     MinScore/Filter/Dimensions/RerankTopN 都按调用传入
//   - TopK 必填；MinScore 为 0 表示不设阈值；RerankTopN 为 0 表示重排后全部保留
//   - Engine 使用调用方 ctx 控制超时
//   - Metric 由 Store 构造配置统一；集合维度在创建时按 IndexRequest.Dimensions 锁定，
//     写入与查询向量都按各自请求的 Dimensions 向量化
//
// # 写入
//
//   - 块主键为 "文档ID#序号"；块内容、切块信息与文档自定义元数据合并写入，保留键见
//     MetadataKeyDocumentID 等常量，Document.Metadata 不使用保留键
//   - 块原文同时写入向量库 Text 字段，供混合检索的 BM25 一路命中
//   - 每块携带文档当前版本（rag_version）与块总数（rag_chunk_count），检索侧据此识别旧块
//   - 每篇文档逐篇写入：全部文档切块与校验通过后，逐篇向量化一次，经 Store.ReplaceByFilter
//     覆盖同序号块并清理多余旧块；同一文档的替换在 Store 实例内串行
//   - 写入失败时旧版本保持完整；中途失败时已写入的文档保留，重试即可补全
//   - 跨实例并发写同一文档时，读侧按文档版本过滤旧块，重试后数据一致
//
// # 检索增强
//
// Retrieve 依次执行检索、精重排与提示词拼接：
//
//   - 检索由 ai/retrieval 执行：查询向量化、hybrid（默认）/ vector 策略、前置元数据
//     过滤、MinScore 阈值与后置域过滤；检索入参非法透传其哨兵
//   - 域过滤由 ai/retrieval 的文档版本过滤器完成，本包提供保留键与首块主键：缺少块
//     文本的命中忽略，版本不一致或序号超出块总数的旧块丢弃，辅助查询失败时放行并告警
//   - 精重排把过滤后的块文本交给 Reranker，保留前 RerankTopN 条，无命中时跳过；
//     结果经 rerank.Apply 按下标回取并回填 RerankScore，下标越界为 rerank.ErrBadResponse
//   - 拼接把重排后的块（Content + Source）交给 Generator，Prompt 直接作为大模型输入；
//     无命中时以空上下文拼接；空提示词为 generation.ErrBadResponse
//   - Chunks 顺序即重排后顺序；Score 是检索/融合分，RerankScore 是重排分（PassThrough 恒 0）
//   - 重排候选超过 rerank.MaxDocs 时由重排契约拒绝，请下调 TopK
//
// # 错误
//
// 本包哨兵：ErrInvalidArgument（配置与编排入参）/ ErrBadResponse（节点返回数据不符合契约）。
// 其余错误透传各组件哨兵：chunking.ErrInvalidArgument（切块契约）、retrieval.ErrInvalidArgument
// （检索入参）、vectordb.ErrInvalidArgument（向量维度）、rerank.ErrBadResponse（重排输出）、
// generation.ErrBadResponse（拼接输出）。
//
// Engine 构造后不可变，可并发调用。
//
// @author zwei
package rag
