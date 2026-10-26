// Package generation 提供把检索到的文档块拼接进输入提示词的门面契约。
//
// 本包只定义 Generator 接口与请求、结果类型，不设 Engine 中间层，也不含提示词模板
// 与大模型接入：拼接策略（模板、引用标注、块排序、超长截断）由业务实现 Generator
// 后自行负责，拼接结果由调用方直接发送给大模型（可配合 component/ai/llm 使用）。
// 入参与输出契约见接口注释；ValidateRequest 与 ValidateResponse 是契约的可执行形式，
// 供实现与调用方复用。内置 Basic 最简拼接实现按固定模板拼接，用于尚未定制提示词
// 模板时的占位。
// 本包不读 YAML、不进插件链、不注册配置根段；实现的资源生命周期由实现自持。
//
// # 基本用法
//
//	// 尚未定制模板：Basic 占位
//	generator := generation.NewBasic()
//	result, err := generator.Generate(ctx, generation.GenerateRequest{
//	    Query: "如何退款",
//	    Contexts: []generation.Context{
//	        {Content: "退款流程为……", Source: "manual.md"},
//	    },
//	})
//	// result.Prompt 直接作为大模型输入
//
//	// 定制模板：实现 Generator；可复用 ValidateRequest 自查入参
//	type myGenerator struct{ /* 模板配置 */ }
//
//	func (g myGenerator) Generate(ctx context.Context, req generation.GenerateRequest) (*generation.GenerateResponse, error) {
//	    if err := generation.ValidateRequest(req); err != nil {
//	        return nil, err
//	    }
//	    prompt := render(g.tpl, req.Query, req.Contexts)
//	    return &generation.GenerateResponse{Prompt: prompt}, nil
//	}
//
// # 契约
//
//   - GenerateRequest 全部字段按调用传入；Query 与每条 Context.Content trim 后非空；
//     Contexts 可为空（无检索命中时是否仍拼接由实现决定，如仅按问题直答）
//   - Prompt trim 后非空；格式与口径由实现定义，调用方直接作为大模型输入
//   - 实现须并发安全，不内置超时，使用调用方 ctx 控制
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument（请求未通过契约校验）/
// ErrBadResponse（拼接实现返回的数据不符合本包契约）。
// 实现自身的业务错误原样返回，可用 errors.Is 判断本包哨兵。
//
// @author zwei
package generation
