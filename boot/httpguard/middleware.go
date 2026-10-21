// Package httpguard 提供 HTTP 请求守卫能力（请求体大小限制与请求级超时）
//
// @author zwei
package httpguard

import (
	"context"
	"net/http"

	"gitee.com/hq_project/yomi-boot/boot/appctx"
	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/handler"
	"gitee.com/hq_project/yomi-boot/boot/router"
	sharederr "gitee.com/hq_project/yomi-boot/boot/shared/error"

	"github.com/gin-gonic/gin"
)

// Middleware 返回基于 HTTP 配置的请求守卫中间件。
// 限制请求体大小，并按 write_timeout - 2s 注入请求级 context deadline。
func Middleware(cfg router.Config) gin.HandlerFunc {
	maxBodyBytes := cfg.MaxRequestBodyBytesOrDefault()
	requestTimeout := cfg.RequestTimeout()

	return func(c *gin.Context) {
		// 守卫1：限制请求体最大读取字节数，超限时由下游 handler.BindJSON 映射为 REQUEST_BODY_TOO_LARGE
		if maxBodyBytes > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		}

		// 守卫2：注入请求级 context deadline（write_timeout - 2s），下游 handler 可通过 ctx.Done() 感知超时
		var cancel context.CancelFunc
		if requestTimeout > 0 {
			var ctx context.Context
			ctx, cancel = appctx.WithTimeout(c.Request.Context(), requestTimeout)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
		}

		c.Next()

		// 后置兜底：若 handler 未写入响应且 context 已超时，回写超时错误
		if requestTimeout <= 0 || c.Writer.Written() {
			return
		}
		if errors.Is(c.Request.Context().Err(), context.DeadlineExceeded) {
			handler.WriteBusinessError(c, errors.Sentinel(sharederr.ErrRequestTimeout))
		}
	}
}
