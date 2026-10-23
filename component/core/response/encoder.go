// Package response 定义 HTTP 响应编码器接口
//
// @author zwei
package response

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
)

// Response 是 Encoder 解析结果，核心为 Code / Msg / Data；
// Total 仅分页时由实现填充，无则 0。
type Response struct {
	Code  string
	Msg   string
	Data  json.RawMessage
	Total int64
}

// Encoder 负责业务 HTTP 响应的 JSON 组装与解析。
// 字段形状、成败语义与 HTTP 状态码均由实现决定；脚手架只调用本接口。
// 常规启动经 boot.Options.ResponseEncoder 指定，为空时 boot 注册 DefaultEncoder；
// 低层启动路径须自行 MustRegister，禁止包 init 静默注册。
type Encoder interface {
	EncodeSuccess(c *gin.Context, data any)
	EncodePage(c *gin.Context, items any, total int64)
	EncodeFailure(c *gin.Context, err error)
	// Parse 解析本实现写出的 body；无法识别时 ok=false。
	Parse(body []byte) (resp Response, ok bool)
	// IsSuccess 判断本实现契约下 code 是否表示成功。
	IsSuccess(code string) bool
}
