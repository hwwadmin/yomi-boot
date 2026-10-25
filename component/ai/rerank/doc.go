// Package rerank 提供按查询对候选文本精重排的通用门面。
//
// 覆盖入参校验、实现输出兜底校验、降序排序与 TopN 截断，不认识任何业务元数据语义，
// 也不包含具体重排模型的接入。Config 只保留 Reranker：业务按自己接入的模型实现
// Reranker 接口并注入，为空时使用内置 PassThrough（不打分、不改顺序、Score 恒 0，
// 等价于保序返回 TOP-N），用于尚未接入模型时的默认占位。
// 配置只在 New / MustNew 传入；本包不读 YAML、不进插件链、不注册配置根段；
// Reranker 的资源生命周期由实现自持。
//
// # 基本用法
//
//	// 未接入模型：默认 PassThrough，保序返回 TOP-N
//	engine, err := rerank.New(rerank.Config{})
//	result, err := engine.Rerank(ctx, rerank.RerankRequest{
//	    Query: "如何退款",
//	    Docs:  []string{"退款流程为……", "发货时效为……"},
//	    TopN:  3,
//	})
//	for _, r := range result.Results {
//	    doc := docs[r.Index] // 按下标回取原对象
//	}
//
//	// 接入自有模型：实现 Reranker 并注入
//	engine, err := rerank.New(rerank.Config{Reranker: myReranker})
//
// # 契约
//
//   - RerankRequest 全部字段按调用传入；Query 与每条 Doc trim 后非空；
//     Docs 1~MaxDocs 条，超过请分批；TopN 为 0 表示全部返回，负数非法，超过 len(Docs) 钳制
//   - Reranker 实现返回与 req.Docs 等长的分数列表，与输入下标一一对应，越大越相关；
//     排序、截断与契约兜底由 Engine 统一执行，实现只需按输入顺序打分
//   - Results 按分数降序稳定排序，同分保持输入顺序；Score 口径由实现定义，不可跨实现横比，
//     阈值过滤由调用方自理；PassThrough 的 Score 恒 0，重排结果不可再按分数阈值过滤
//   - Engine 不内置超时，使用调用方 ctx 控制；Reranker 实现须并发安全
//   - Engine 构造后不可变，可并发调用；请勿复制 Engine 值
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument / ErrBadResponse。
// Reranker 实现返回的错误原样透传，可用 errors.Is 判断其哨兵。
//
// @author zwei
package rerank
