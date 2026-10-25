// Package httprecord 测试装配：注册测试响应编码器，供业务码解析链路使用
//
// @author zwei
package httprecord

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"gitee.com/hq_project/yomi-boot/component/core/response"
)

// testResponseEncoder 测试用响应编码器：按 {code,msg} 形状解析业务码
type testResponseEncoder struct{}

// EncodeSuccess 空实现
func (testResponseEncoder) EncodeSuccess(*gin.Context, any) {}

// EncodePage 空实现
func (testResponseEncoder) EncodePage(*gin.Context, any, int64) {}

// EncodeFailure 空实现
func (testResponseEncoder) EncodeFailure(*gin.Context, error) {}

// Parse 解析 {code,msg} 形状，code 为空视为无法识别
func (testResponseEncoder) Parse(body []byte) (response.Response, bool) {
	var parsed struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Code == "" {
		return response.Response{}, false
	}
	return response.Response{Code: parsed.Code, Msg: parsed.Msg}, true
}

// IsSuccess 判断 code 是否表示成功
func (testResponseEncoder) IsSuccess(code string) bool { return code == "OK" }

// TestMain 注册测试编码器并关闭 gin 调试日志
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	response.MustRegister(testResponseEncoder{})
	os.Exit(m.Run())
}
