// 内置最简拼接实现：固定模板占位。
//
// @author zwei

package generation

import (
	"context"
	"strconv"
	"strings"
)

// basic 最简拼接实现：问题原文 + 按序编号的参考资料固定模板
type basic struct{}

var _ Generator = basic{}

// NewBasic 返回最简拼接实现；用于业务尚未定制提示词模板时的占位，
// 拼接格式见 Generate，接入自有模板后替换为自有 Generator 实现
func NewBasic() Generator {
	return basic{}
}

// Generate 实现 Generator；按固定模板拼接，Contexts 为空时 Prompt 即问题原文。
// Prompt 格式（Source 非空时附来源）：
//
//	{Query}
//
//	参考资料：
//	[1] {Content}（来源：{Source}）
//	[2] {Content}
func (basic) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(req.Query))
	if len(req.Contexts) > 0 {
		b.WriteString("\n\n参考资料：")
		for i, c := range req.Contexts {
			b.WriteString("\n[")
			b.WriteString(strconv.Itoa(i + 1))
			b.WriteString("] ")
			b.WriteString(strings.TrimSpace(c.Content))
			if source := strings.TrimSpace(c.Source); source != "" {
				b.WriteString("（来源：")
				b.WriteString(source)
				b.WriteString("）")
			}
		}
	}
	return &GenerateResponse{Prompt: b.String()}, nil
}
