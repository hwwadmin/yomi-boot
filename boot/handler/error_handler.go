// Package handler 提供 handler 层通用协议适配能力
//
// @author zwei
package handler

import (
	"context"
	"net/http"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/logger"
	"gitee.com/hq_project/yomi-boot/boot/response"
	sharederr "gitee.com/hq_project/yomi-boot/boot/shared/error"
	"gitee.com/hq_project/yomi-boot/boot/utils/ginutil"

	"github.com/gin-gonic/gin"
)

// WriteBusinessError 输出失败响应
func WriteBusinessError(c *gin.Context, err error) {
	WriteBusinessErrorWithFallback(c, err, nil)
}

// WriteBusinessErrorWithFallback 输出失败响应；无法识别的 err 在提供 fallback 时改用 fallback
func WriteBusinessErrorWithFallback(c *gin.Context, err error, fallback error) {
	out := normalizeFailure(err, fallback)
	logFailure(c, out)
	response.Enc().EncodeFailure(c, out)
}

// LogRequestError 记录接口请求异常日志
func LogRequestError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	logFailure(c, normalizeFailure(err, nil))
}

// normalizeFailure 将超时/超限等归一为框架哨兵，其余原样上抛（不投影 code）
func normalizeFailure(err, fallback error) error {
	if err == nil {
		if fallback != nil {
			return errors.Sentinel(fallback)
		}
		return errors.Sentinel(sharederr.ErrInternal)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return errors.Sentinel(sharederr.ErrRequestTimeout)
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return errors.Sentinel(sharederr.ErrRequestBodyTooLarge)
	}

	var biz *errors.BizError
	if errors.As(err, &biz) {
		return err
	}

	if fallback != nil && !errors.Is(err, fallback) {
		return errors.Sentinel(fallback)
	}

	return err
}

// logFailure 记录接口失败日志，附带请求信息、业务码与堆栈
func logFailure(c *gin.Context, err error) {
	if err == nil {
		return
	}

	keysAndValues := []any{
		"error", err,
		"stack", errors.Stack(err),
	}
	var biz *errors.BizError
	if errors.As(err, &biz) {
		keysAndValues = append(keysAndValues, "code", biz.Code, "message", biz.Message)
	}

	logCtx := context.Background()
	if r := c.Request; r != nil {
		logCtx = r.Context()
		keysAndValues = append([]any{
			"method", r.Method,
			"path", r.URL.Path,
			"clientIP", ginutil.ClientIP(c),
		}, keysAndValues...)
		if r.URL.RawQuery != "" {
			keysAndValues = append(keysAndValues, "query", r.URL.RawQuery)
		}
	}
	logger.Error(logCtx, "接口请求异常", keysAndValues...)
}
