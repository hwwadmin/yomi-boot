// 定义调用、结果与批量结局类型。
//
// @author zwei

package tool

import "encoding/json"

// Call 一次工具调用；ID 由模型或调用方提供，原样回填到 Result，可用于日志与幂等
type Call struct {
	ID        string          `json:"id,omitempty"` // ID 是调用标识，可空
	Name      string          `json:"name"`         // Name 是工具名；查找前 trim
	Arguments json.RawMessage `json:"arguments"`    // Arguments 是 JSON 对象；空或 null 归一为 {}
	Evidence  *Evidence       `json:"-"`            // Evidence 是宿主侧填槽证据；不序列化、不发给模型
}

// Evidence 填槽证据；由调用方收集，库只用于拼提示词与标识符门闩
type Evidence struct {
	UserInput string         `json:"user_input"`        // UserInput 是本轮用户原文
	History   []EvidenceTurn `json:"history,omitempty"` // History 是同会话历史轮次
	OCR       []string       `json:"ocr,omitempty"`     // OCR 是附件识别文本片段
	Locale    string         `json:"locale,omitempty"`  // Locale 是语种，如 zh-CN
}

// EvidenceTurn 一条历史轮次
type EvidenceTurn struct {
	Role    string `json:"role"`    // Role 取值 user / assistant
	Content string `json:"content"` // Content 是截断后正文
}

// Result 一次工具调用的结果
type Result struct {
	CallID  string          `json:"call_id,omitempty"` // CallID 回指 Call.ID
	Name    string          `json:"name"`              // Name 是注册时的规范工具名
	Content json.RawMessage `json:"content"`           // Content 是合法 JSON；无输出为 null
}

// Outcome 批量执行中的一个调用结局；成功时 Result 非空且 Err 为空，失败时相反
type Outcome struct {
	Call   Call    `json:"call"`             // Call 是调用方传入的原始调用
	Result *Result `json:"result,omitempty"` // Result 是执行结果；失败时为 nil
	Err    error   `json:"err,omitempty"`    // Err 实际总是 *Error；失败时非 nil
}
