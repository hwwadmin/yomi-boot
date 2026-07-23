// Package response 提供默认 Encoder（当前 API 风格预制实现，须显式 MustRegister）
//
// @author zwei
package response

import (
	"encoding/json"
	"gitee.com/hq_project/yomi-boot/errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// DefaultEncoder 当前 API 风格的预制 Encoder，须业务 main 显式 MustRegister。
// 写出：{code,msg,timestamp,data}，分页另含顶层 total；HTTP 固定 200。
// 失败：code/msg 直接取自 err（*BizError 取其 Code/Message，其余取 Error()）。
type DefaultEncoder struct{}

var _ Encoder = DefaultEncoder{}

// defaultBody 默认 API 响应 JSON 结构
type defaultBody struct {
	Code      string          `json:"code"`
	Msg       string          `json:"msg"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
	Total     int64           `json:"total,omitempty"`
}

// EncodeSuccess 编码成功响应（code=OK）。
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

// EncodePage 编码分页成功响应（code=OK，total 在顶层）。
func (DefaultEncoder) EncodePage(c *gin.Context, items any, total int64) {
	c.JSON(http.StatusOK, map[string]any{
		"code":      "OK",
		"msg":       "success",
		"timestamp": time.Now().UnixMilli(),
		"data":      items,
		"total":     total,
	})
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

// Parse 解析当前 API 风格 body → code / msg / data / total。
func (DefaultEncoder) Parse(body []byte) (Response, bool) {
	var raw defaultBody
	if err := json.Unmarshal(body, &raw); err != nil {
		return Response{}, false
	}
	if raw.Code == "" && raw.Msg == "" {
		return Response{}, false
	}
	return Response{
		Code:  raw.Code,
		Msg:   raw.Msg,
		Data:  raw.Data,
		Total: raw.Total,
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
