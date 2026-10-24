package handler

import (
	"context"
	"net/http"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/response"
	"gitee.com/hq_project/yomi-boot/component/core/shared/dto"
	sharederr "gitee.com/hq_project/yomi-boot/component/core/shared/error"

	"github.com/gin-gonic/gin"
)

// BindJSON 绑定请求体 JSON，失败时写入业务异常并返回 false
func BindJSON(c *gin.Context, input any) bool {
	if err := c.ShouldBindJSON(input); err != nil {
		WriteBusinessError(c, JSONBindError(err))
		return false
	}

	return true
}

// JSONBindError 将 JSON 绑定错误映射为业务异常
func JSONBindError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return errors.Sentinel(sharederr.ErrRequestBodyTooLarge)
	}
	return errors.Sentinel(sharederr.ErrInvalidBody)
}

// Handle 执行返回数据的用例并输出成功响应
func Handle[T any](c *gin.Context, fn func(ctx context.Context) (T, error)) {
	output, err := fn(c.Request.Context())
	if err != nil {
		WriteBusinessError(c, err)
		return
	}

	WriteSuccess(c, output)
}

// HandlePage 执行分页用例并输出分页成功响应
func HandlePage[T any](c *gin.Context, fn func(ctx context.Context) (*dto.PageOutput[T], error)) {
	output, err := fn(c.Request.Context())
	if err != nil {
		WriteBusinessError(c, err)
		return
	}
	if output == nil {
		WriteBusinessError(c, errors.Sentinel(sharederr.ErrInternal))
		return
	}

	WritePageSuccess(c, output.Items, output.Total)
}

// HandleVoid 执行无返回数据的用例并输出成功响应
func HandleVoid(c *gin.Context, fn func(ctx context.Context) error) {
	if err := fn(c.Request.Context()); err != nil {
		WriteBusinessError(c, err)
		return
	}

	WriteSuccess(c, nil)
}

// BindAndHandle 绑定请求体后执行返回数据的用例并输出成功响应
func BindAndHandle[TIn any, TOut any](c *gin.Context, execute func(ctx context.Context, input *TIn) (TOut, error)) {
	var input TIn
	if !BindJSON(c, &input) {
		return
	}

	Handle(c, func(ctx context.Context) (TOut, error) {
		return execute(ctx, &input)
	})
}

// BindAndHandlePage 绑定请求体后执行分页用例并输出分页成功响应
func BindAndHandlePage[TIn any, TItem any](c *gin.Context, execute func(ctx context.Context, input *TIn) (*dto.PageOutput[TItem], error)) {
	var input TIn
	if !BindJSON(c, &input) {
		return
	}

	HandlePage(c, func(ctx context.Context) (*dto.PageOutput[TItem], error) {
		return execute(ctx, &input)
	})
}

// BindAndHandleVoid 绑定请求体后执行无返回数据的用例并输出成功响应
func BindAndHandleVoid[TIn any](c *gin.Context, execute func(ctx context.Context, input *TIn) error) {
	var input TIn
	if !BindJSON(c, &input) {
		return
	}

	HandleVoid(c, func(ctx context.Context) error {
		return execute(ctx, &input)
	})
}

// BindAndHandleWithRequest 绑定请求体后执行需要 HTTP 请求对象的用例并输出成功响应
func BindAndHandleWithRequest[TIn any, TOut any](c *gin.Context, execute func(ctx context.Context, req *http.Request, input *TIn) (TOut, error)) {
	var input TIn
	if !BindJSON(c, &input) {
		return
	}

	Handle(c, func(ctx context.Context) (TOut, error) {
		return execute(ctx, c.Request, &input)
	})
}

// WriteSuccess 输出成功响应
func WriteSuccess(c *gin.Context, data any) {
	response.Enc().EncodeSuccess(c, data)
}

// WritePageSuccess 输出分页成功响应
func WritePageSuccess(c *gin.Context, data any, total int64) {
	response.Enc().EncodePage(c, data, total)
}

// WriteFailure 输出失败响应
func WriteFailure(c *gin.Context, err error) {
	WriteBusinessError(c, err)
}
