// 定义验证码哨兵错误。
//
// @author zwei

package captcha

import "github.com/hwwadmin/yomi-boot/component/core/errors"

var (
	// ErrInvalidArgument 配置或调用参数未通过本包校验
	ErrInvalidArgument = errors.NewNoStack("CAPTCHA_INVALID_ARGUMENT")
	// ErrExpired 验证码不存在、已过期或已被消费
	ErrExpired = errors.NewNoStack("CAPTCHA_EXPIRED")
	// ErrMismatch 验证码答案不匹配；校验时已消费
	ErrMismatch = errors.NewNoStack("CAPTCHA_MISMATCH")
	// ErrGenerateFailed 渲染或编码验证码失败
	ErrGenerateFailed = errors.NewNoStack("CAPTCHA_GENERATE_FAILED")
)

// invalidArgument 返回带 ErrInvalidArgument 身份的配置/调用参数错误
func invalidArgument(format string, args ...any) error {
	return errors.Wrap(ErrInvalidArgument, format, args...)
}
