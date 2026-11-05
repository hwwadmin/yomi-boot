package handler

import (
	"context"
	"net/http"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/logger"
	"github.com/hwwadmin/yomi-boot/component/core/response"
	sharederr "github.com/hwwadmin/yomi-boot/component/core/shared/error"
	"github.com/hwwadmin/yomi-boot/component/core/utils/ginutil"

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

// normalizeFailure 将超时/超限等收成框架哨兵供对外 code 使用。
// 哨兵不携带原错误；这里用 errors.WithCode 保住原因链，日志仍打收口前的错误。
func normalizeFailure(err, fallback error) error {
	if err == nil {
		if fallback != nil {
			return errors.Sentinel(fallback)
		}
		return errors.Sentinel(sharederr.ErrInternal)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return errors.WithCode(sharederr.ErrRequestTimeout, err)
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return errors.WithCode(sharederr.ErrRequestBodyTooLarge, err)
	}

	var biz *errors.BizError
	if errors.As(err, &biz) {
		return err
	}

	if fallback != nil && !errors.Is(err, fallback) {
		return errors.WithCode(fallback, err)
	}

	return err
}

// logFailure 记录接口失败日志，附带请求信息、业务码与堆栈。
// 收口为哨兵时，error/stack 打原始原因，code 仍为对外哨兵。
func logFailure(c *gin.Context, err error) {
	if err == nil {
		return
	}

	logged := err
	sentinel, cause, collapsed := errors.PublicCode(err)
	if collapsed && cause != nil {
		logged = cause
	}

	keysAndValues := []any{
		"error", logged,
		"stack", errors.Stack(logged),
	}
	var biz *errors.BizError
	if errors.As(logged, &biz) {
		keysAndValues = append(keysAndValues, "code", biz.Code, "message", biz.Message)
	} else if collapsed && sentinel != nil {
		if code := sentinel.Error(); code != "" && code != logged.Error() {
			keysAndValues = append(keysAndValues, "code", code)
		}
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
			keysAndValues = append(keysAndValues, "query", logger.MaskQuery(r.URL.RawQuery))
		}
	}
	logger.Error(logCtx, "接口请求异常", keysAndValues...)
}
