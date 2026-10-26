// Package rag 提供两条标准化 RAG 流程的编排：文档入库与文档检索增强。
//
// 本包只做流程编排，不实现任何流程节点：切块、向量化、检索、精重排与提示词拼接
// 分别由 component/ai/chunking、ai/embedding、ai/retrieval、ai/rerank、ai/generation
// 的门面负责，构造时经 Config 全部注入（全部必填）；本包在节点之间补齐文档域语义——
// 写入侧的块主键、保留元数据键、文档版本与旧块清理，检索侧的旧块过滤。
// 向量库 Store 是基础设施依赖（写入与域过滤辅助查询），不是流程节点。
// 门面接口见 Engine，由 New / MustNew 构造。
// 同一个 Engine 可并发服务多个集合（不同业务使用不同集合名即可）。
// 配置只在 New / MustNew 传入；本包不读 YAML、不进插件链、不注册配置根段。
// 本包不持有各依赖的关闭权，进程退出时由调用方自行关闭。
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
//	err = engine.DeleteDocument(ctx, "kb_sales", "doc-1")
//
// # 两条流程
//
//	路径① 文档入库：Document → Chunker 切块 → Embedder 向量化 → Store 写入；
//	  本包负责块主键（"文档ID#序号"）、保留元数据键、文档版本与旧块清理、
//	  条带锁与按块预算分批写入。
//	路径② 文档检索增强：Query → Retriever 检索（含域过滤）→ Reranker 精重排
//	  → Generator 拼接提示词；返回 Prompt（直接交大模型）与重排后的 Chunks。
//
// 装配约束：Retriever 须与 Store 基于同一向量库，其查询向量化须与 Embedder
// 同模型同维度；不一致时写入与检索将互不相认或由 vectordb 校验报错
// （记录与查询向量维度由 vectordb 按集合 Dimension 校验）。
//
// # 多集合
//
// 集合名每次调用传入；Index 对不存在的集合自动创建，Retrieve 在集合不存在时返回
// 空 Chunks 与由 Generator 拼接的 Prompt，DeleteDocument 在集合不存在时返回 nil。
// 不同业务共用同一个 Engine 时，用不同集合名隔离数据即可；所有集合共用同一
// Store 的向量维度与 Metric。
//
// # 切块
//
// Config.Chunker 必填，内置策略 recursive / fixed / sentence / markdown 及其配置见
// component/ai/chunking；IndexRequest.Chunker 可按调用覆盖（如 Markdown 文档与
// 纯文本走不同策略）。也可实现 chunking.Chunker 注入自定义策略，Engine 对其输出
// 兜底校验，违反契约透传 chunking.ErrInvalidArgument。
// 块长按 rune 计，单块不得超过 chunking.MaxChunkSize 字。
//
// # 运行期参数
//
//   - 集合名、IndexRequest.Chunker、RetrieveRequest.Strategy/TopK/MinScore/Filter/
//     RerankTopN 都按调用传入
//   - RetrieveRequest.TopK 必填；MinScore 为 0 表示不设阈值；RerankTopN 为 0 表示
//     重排后全部保留
//   - Engine 不内置超时，使用调用方 ctx 控制
//   - Store 的维度与 Metric 由 Store 构造配置决定，其下所有集合一致
//
// # 写入
//
//   - 块主键为 "文档ID#序号"；内容与切分信息写入保留元数据键，文档自定义元数据原样合并
//   - 块原文同时写入向量库 Text 字段，供混合检索的 BM25 一路命中；不额外占向量维度
//   - 每个块写入文档当前版本（保留键 rag_version）与块总数（rag_chunk_count），供检索侧过滤旧块
//   - 同一文档重复 Index 会覆盖同序号块，并在写入成功后删除该文档中不属于本次新块集合的旧块
//   - Index 按块预算（256 块）分批写入，避免一次请求把全部向量与记录同时放在内存；
//     向量化与主键分批由 embedding / vectordb 内部承担；中途失败时已写入的批次保留，重试即可
//   - 旧块清理依据保留键 rag_doc_id 列出该文档现有块，不依赖块数元数据
//   - 同一文档的写入阶段由库内条带锁串行；条带锁仅保证进程内，跨实例需调用方用分布式锁或串行投递
//   - Upsert 失败时不会删除旧块（可能夹杂已写入的新块）；检索按当前版本与块总数过滤，正常情况不会返回旧版本块，重试可自愈
//   - 不同文档、不同集合可并发；条带碰撞会短暂串行
//   - 保留元数据键见 MetadataKeyDocumentID 等常量，Document.Metadata 不得使用
//
// # 检索增强
//
// 编排顺序：检索（含三层过滤）→ 精重排 → 提示词拼接。
//
//   - 检索段由 component/ai/retrieval 门面执行（入参校验、查询向量化、策略路由、
//     前置过滤、阈值过滤与后置过滤），检索入参非法透传其哨兵；hybrid（默认）/
//     vector 的策略口径见其文档
//   - 域过滤是检索链路的后置过滤：本包实现 retrieval.HitFilter 并经
//     RetrieveRequest.PostFilter 交给检索层执行——缺少块文本的命中忽略；读取命中
//     文档当前版本与块总数，丢弃版本不一致或索引超出总数的旧块；辅助查询失败时
//     降级为不过滤并告警。RetrieveRequest.PostFilter 由本包固定注入，调用方传值
//     会被覆盖；该过滤器不导出，直接使用 ai/retrieval 检索本包写入的集合不会做旧块过滤
//   - 精重排：过滤后的块文本交由 Reranker，TopN 取 RerankTopN；无命中时跳过
//     （重排契约要求至少 1 条候选）；结果经 rerank.Apply 按下标回取并回填
//     RerankScore，下标越界、nil 响应等违约为 rerank.ErrBadResponse；
//     重排候选超过 rerank.MaxDocs 时由重排契约拒绝（透传 AI_RERANK_INVALID_ARGUMENT），
//     请下调 TopK
//   - 拼接：重排后的块（Content + Source）交由 Generator，Prompt 直接作为大模型输入；
//     检索无命中时以空上下文拼接，拼接方式由实现决定；输出经
//     generation.ValidateResponse 核对，空提示词为 generation.ErrBadResponse
//   - Chunks 顺序 = 重排后顺序（RerankScore 口径由 Reranker 定义，PassThrough 恒 0，
//     顺序即检索顺序）；Score 是检索/融合分，与 RerankScore 不可横比
//   - Score 越大越相似；不同 Metric 与不同 Strategy 的 Score 不可横比
//   - DeleteDocument：删除文档全部块；集合不存在时返回 nil
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument（本包配置与编排入参）/ ErrBadResponse（节点实现
// 返回数据不符合其门面契约时由本包兜底识别，如向量化返回条数与输入不一致）。
// chunking / embedding / vectordb / retrieval / rerank / generation 的错误原样透传，
// 可用 errors.Is 判断其哨兵：切块契约→chunking.ErrInvalidArgument，检索入参→
// retrieval.ErrInvalidArgument，查询与写入向量维度→vectordb.ErrInvalidArgument，
// 重排输出违约→rerank.ErrBadResponse，拼接输出违约→generation.ErrBadResponse。
//
// Engine 构造后不可变，可并发调用；请勿复制实现值。
//
// @author zwei
package rag
