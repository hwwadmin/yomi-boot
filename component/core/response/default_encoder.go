package response

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/errors"

	"github.com/gin-gonic/gin"
)

// DefaultEncoder 当前 API 风格的预制 Encoder，boot.Options.ResponseEncoder
// 为空时由 boot 默认注册。
// 写出：{code,msg,timestamp,data}；HTTP 固定 200。
// 失败：code/msg 直接取自 err（*BizError 取其 Code/Message，其余取 Error()）。
type DefaultEncoder struct{}

var _ Encoder = DefaultEncoder{}

// defaultBody 默认 API 响应 JSON 结构
type defaultBody struct {
	Code      string          `json:"code"`
	Msg       string          `json:"msg"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// EncodeSuccess 编码成功响应（code=OK）。
// 分页等业务载荷形状由业务出参决定，随 data 落地。
func (DefaultEncoder) EncodeSuccess(c *gin.Context, data any) {
	body := map[string]any{
		"code":      "OK",
		"msg":       "success",
		"timestamp": time.Now().UnixMilli(),
	}
	if data != nil {
		body["data"] = data
	}
	c.JSON(http.StatusOK, body)
}

// EncodeFailure 编码失败响应；code/msg 直接来自 err。
func (DefaultEncoder) EncodeFailure(c *gin.Context, err error) {
	code, msg := wireFromErr(err)
	c.JSON(http.StatusOK, map[string]any{
		"code":      code,
		"msg":       msg,
		"timestamp": time.Now().UnixMilli(),
	})
}

// Parse 解析当前 API 风格 body → code / msg / data。
func (DefaultEncoder) Parse(body []byte) (Response, bool) {
	var raw defaultBody
	if err := json.Unmarshal(body, &raw); err != nil {
		return Response{}, false
	}
	if raw.Code == "" && raw.Msg == "" {
		return Response{}, false
	}
	return Response{
		Code: raw.Code,
		Msg:  raw.Msg,
		Data: raw.Data,
	}, true
}

// IsSuccess 当前风格下 code=OK 表示成功。
func (DefaultEncoder) IsSuccess(code string) bool {
	return code == "OK"
}

// wireFromErr 从 error 提取响应 code 与 msg
func wireFromErr(err error) (code, msg string) {
	if err == nil {
		return "INTERNAL_ERROR", "INTERNAL_ERROR"
	}
	var biz *errors.BizError
	if errors.As(err, &biz) && biz != nil && biz.Code != "" {
		return biz.Code, biz.Message
	}
	s := err.Error()
	return s, s
}
