// 定义拼接请求与结果类型。
//
// @author zwei

package generation

// GenerateRequest 提示词拼接入参
type GenerateRequest struct {
	Query    string    // Query 是用户问题；trim 后非空
	Contexts []Context // Contexts 是检索到的文档块；可为空（无检索命中时由实现决定拼接方式）；按传入顺序拼接
}

// Context 一条拼接进提示词的文档块
type Context struct {
	Content string // Content 是块文本；trim 后非空
	Source  string // Source 是块来源（URL / 路径等），可选；供实现做引用标注
}

// GenerateResponse 提示词拼接输出
type GenerateResponse struct {
	Prompt string // Prompt 是拼接后的完整输入提示词；trim 后非空，调用方直接作为大模型输入
}
