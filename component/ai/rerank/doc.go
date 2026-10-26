// Package rerank 提供按查询对候选文本精重排的门面契约。
//
// 本包只定义 Reranker 接口与请求、结果类型，不设 Engine 中间层，也不包含具体重排模型
// 的接入：业务按自己接入的模型实现 Reranker 后直接使用。入参与输出契约见接口注释；
// ValidateRequest、RankScores 与 Apply 是契约的可执行形式，供实现与调用方复用——
// 前者校验请求，RankScores 把按输入顺序的分数整理为降序结果（最常见的实现形态：
// 调上游重排服务拿到等长分数后转换），Apply 把结果按下标应用到调用方自有候选切片。
// 内置 PassThrough 空实现不打分、不改顺序，等价于保序返回 TOP-N，
// 用于尚未接入模型时的占位。
// 本包不读 YAML、不进插件链、不注册配置根段；实现的资源生命周期由实现自持。
//
// # 基本用法
//
//	// 尚未接入模型：PassThrough 占位，保序返回 TOP-N
//	reranker := rerank.NewPassThrough()
//	result, err := reranker.Rerank(ctx, rerank.RerankRequest{
//	    Query: "如何退款",
//	    Docs:  []string{"退款流程为……", "发货时效为……"},
//	    TopN:  3,
//	})
//	for _, r := range result.Results {
//	    doc := docs[r.Index] // 按下标回取原对象
//	}
//
//	// 接入自有模型：实现 Reranker；按输入顺序打分的实现可直接复用 RankScores
//	type myReranker struct{}
//
//	func (myReranker) Rerank(ctx context.Context, req rerank.RerankRequest) (*rerank.RerankResponse, error) {
//	    if err := rerank.ValidateRequest(req); err != nil {
//	        return nil, err
//	    }
//	    scores, err := callUpstream(ctx, req.Query, req.Docs)
//	    if err != nil {
//	        return nil, err
//	    }
//	    results, err := rerank.RankScores(scores, req.TopN)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &rerank.RerankResponse{Results: results, Tokens: tokens}, nil
//	}
//
// # 契约
//
//   - RerankRequest 全部字段按调用传入；Query 与每条 Doc trim 后非空；
//     Docs 1~MaxDocs 条，超过请分批；TopN 为 0 表示全部返回，负数非法，超过 len(Docs) 钳制
//   - Results 按分数降序稳定排序，同分保持输入顺序；Score 口径由实现定义，不可跨实现横比，
//     阈值过滤由调用方自理；PassThrough 的 Score 恒 0，重排结果不可再按分数阈值过滤；
//     Index 必须回指请求 Docs 下标，调用方可用 Apply 回取自有对象（越界视为违约）
//   - 实现须并发安全，不内置超时，使用调用方 ctx 控制
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument（请求或 topN 未通过契约校验）/
// ErrBadResponse（分数含 NaN/Inf）。实现自身的业务错误原样返回，可用 errors.Is 判断本包哨兵。
//
// @author zwei
package rerank
