// Package dtm 提供 DTM 分支处理辅助函数
//
// @author zwei
package dtm

import (
	"context"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/handler"
	"net/http"

	"github.com/dtm-labs/client/dtmcli"
	"github.com/gin-gonic/gin"
)

// ErrNotConfigured 表示 DTM 服务器地址未配置
var ErrNotConfigured = errors.NewNoStack("dtm server 未配置")

// ErrGenGidFailed 表示向 DTM 请求全局事务 ID 失败
var ErrGenGidFailed = errors.NewNoStack("dtm 生成全局事务 ID 失败")

// WriteSuccess 写入 DTM 分支成功响应
func WriteSuccess(c *gin.Context, data any) {
	if data == nil {
		c.JSON(http.StatusOK, gin.H{"dtm_result": dtmcli.ResultSuccess})
		return
	}
	c.JSON(http.StatusOK, gin.H{"dtm_result": dtmcli.ResultSuccess, "data": data})
}

// WriteFailure 写入 DTM 分支失败响应，会尝试从错误链中提取 BizError 的 Code
func WriteFailure(c *gin.Context, err error) {
	if err != nil {
		handler.LogRequestError(c, err)
	}
	resp := gin.H{"dtm_result": dtmcli.ResultFailure}
	if err != nil {
		resp["message"] = err.Error()
		var appErr *errors.BizError
		if errors.As(err, &appErr) {
			resp["code"] = appErr.Code
		}
	}
	c.JSON(http.StatusConflict, resp)
}

// BindAndHandleAction 绑定 JSON 请求体、附加 Barrier、执行业务正向操作
func BindAndHandleAction[TIn any, TOut any](c *gin.Context, execute func(ctx context.Context, input *TIn) (TOut, error)) {
	var input TIn
	if err := c.ShouldBindJSON(&input); err != nil {
		WriteFailure(c, handler.JSONBindError(err))
		return
	}
	ctx, err := GinContextWithBarrier(c)
	if err != nil {
		WriteFailure(c, err)
		return
	}
	output, err := execute(ctx, &input)
	if err != nil {
		WriteFailure(c, err)
		return
	}
	WriteSuccess(c, output)
}

// BindAndHandleCompensate 绑定 JSON 请求体、附加 Barrier、执行业务补偿操作
func BindAndHandleCompensate[TIn any](c *gin.Context, execute func(ctx context.Context, input *TIn) error) {
	var input TIn
	if err := c.ShouldBindJSON(&input); err != nil {
		WriteFailure(c, handler.JSONBindError(err))
		return
	}
	ctx, err := GinContextWithBarrier(c)
	if err != nil {
		WriteFailure(c, err)
		return
	}
	if err := execute(ctx, &input); err != nil {
		WriteFailure(c, err)
		return
	}
	WriteSuccess(c, nil)
}
