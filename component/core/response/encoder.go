package response

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
)

// Response 是 Encoder 解析结果：Code / Msg / Data。
type Response struct {
	Code string
	Msg  string
	Data json.RawMessage
}

// Encoder 负责业务 HTTP 响应的 JSON 组装与解析。
// 只关心信封本身（成败与 data 载荷），分页等业务载荷形状由业务出参决定。
// 字段形状、成败语义与 HTTP 状态码均由实现决定；脚手架只调用本接口。
// 常规启动经 boot.Options.ResponseEncoder 指定，为空时 boot 注册 DefaultEncoder；
// 低层启动路径须自行 MustRegister，禁止包 init 静默注册。
type Encoder interface {
	EncodeSuccess(c *gin.Context, data any)
	EncodeFailure(c *gin.Context, err error)
	// Parse 解析本实现写出的 body；无法识别时 ok=false。
	Parse(body []byte) (resp Response, ok bool)
	// IsSuccess 判断本实现契约下 code 是否表示成功。
	IsSuccess(code string) bool
}
