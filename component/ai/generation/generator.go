// 门面接口定义。
//
// @author zwei

package generation

import "context"

// Generator 提示词拼接门面：把用户问题与检索到的文档块拼接为最终输入提示词。
// 拼接策略（模板、引用标注、块排序、超长截断）由实现负责；实现须并发安全
type Generator interface {
	// Generate 把检索到的文档块拼接到输入提示词
	//
	// 入参契约：Query 与每条 Context.Content trim 后非空；Contexts 可为空。
	// 输出契约：Prompt 为拼接后的完整输入提示词，trim 后非空，调用方直接作为
	// 大模型输入；不得修改 req。
	// 实现可经 ValidateRequest 复用契约校验。
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}
