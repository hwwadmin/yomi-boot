// Package retrieval 提供组合 ai/embedding 与 ai/vectordb 的通用检索门面。
//
// 覆盖查询向量化、纯向量与向量 + BM25 混合检索、阈值过滤，不认识任何业务元数据语义，
// 也不包含大模型问答。Config 只保留 Embedder 与 Store；集合名与运行期参数都按调用传入，
// 同一个 Engine 可并发服务多个集合。
// 配置只在 New / MustNew 传入；本包不读 YAML、不进插件链、不注册配置根段。
// 本包不持有 Embedder / Store 的关闭权，进程退出时由调用方自行关闭。
//
// # 基本用法
//
//	engine, err := retrieval.New(retrieval.Config{Embedder: emb, Store: store})
//	hits, err := engine.Retrieve(ctx, "kb_sales", retrieval.RetrieveRequest{
//	    Query: "如何退款",
//	    TopK:  5,
//	})
//
// # 检索策略
//
// RetrieveRequest.Strategy 选择检索策略，空则 hybrid：
//
//   - hybrid（默认）：向量 + BM25 混合检索，查询原文同时交给向量库的词法一路，
//     两路结果以 RRF（k=60）融合；Score 是融合排名分（量级约 0 ~ 2/61），
//     MinScore 阈值须按融合分口径设置，与 vector 的 Score 不可横比
//   - vector：纯向量检索，Score 是相似度（越大越相似，L2 已映射）
//
// 词法能力由 vectordb 提供（milvus 原生 BM25 全文检索要求 Milvus 2.5+，
// 旧版本创建的集合须重建后才能混合检索；memory 自带 BM25 实现），详见 component/ai/vectordb 文档。
//
// # 过滤
//
// 检索链路上的过滤全部由本包执行，分三层：
//
//   - 前置过滤：RetrieveRequest.Filter（元数据等值 AND）随检索下推到向量库
//   - 阈值过滤：RetrieveRequest.MinScore 在检索返回后按分数过滤，0 表示不设阈值
//   - 后置过滤：RetrieveRequest.PostFilter 是 HitFilter 扩展点，在阈值过滤后应用，
//     供调用方叠加域语义（如文档版本治理、内容完整性）；nil 表示不过滤，
//     返回 vectordb.ErrNotFound 等同于集合消失（按空结果处理），其他错误原样上抛
//
// # 契约
//
//   - 集合名、RetrieveRequest 全部字段按调用传入；TopK 必填；MinScore 为 0 表示不设阈值
//   - 集合不存在或为空时返回空切片与 nil，不视为错误，且不消耗向量化调用
//   - Engine 不内置超时，使用调用方 ctx 控制；查询向量维度由 vectordb 校验兜底，
//     检索路径不做写侧的维度锁定
//   - Engine 构造后不可变，可并发调用；请勿复制 Engine 值
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument / ErrBadResponse。
// embedding / vectordb 的错误原样透传，可用 errors.Is 判断其哨兵。
//
// @author zwei
package retrieval
