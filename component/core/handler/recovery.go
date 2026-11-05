package handler

import (
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	sharederr "github.com/hwwadmin/yomi-boot/component/core/shared/error"

	"github.com/gin-gonic/gin"
)

// Recovery 接住后续中间件和 handler 的 panic，按统一失败信封回写。
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		if c.Writer != nil && c.Writer.Written() {
			c.Abort()
			return
		}
		WriteBusinessError(c, errors.Sentinel(sharederr.ErrInternal))
		c.Abort()
	})
}
