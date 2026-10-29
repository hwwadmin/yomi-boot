// Config 到 base64Captcha Driver 的适配；上游类型只在本文件与 service.go 内部使用。
//
// @author zwei

package captcha

import (
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	base64Captcha "github.com/mojocn/base64Captcha"
)

const (
	digitMaxSkew  = 0.7
	digitDotCount = 80
)

// newDriver 按配置构造上游驱动；Driver 构造一次后复用，不能每请求重建
func newDriver(cfg Config) (driver base64Captcha.Driver, err error) {
	defer func() {
		if r := recover(); r != nil {
			driver = nil
			err = errors.Wrap(ErrGenerateFailed, "captcha: 构造驱动失败: %v", r)
		}
	}()

	switch cfg.Driver {
	case DriverDigit:
		driver = base64Captcha.NewDriverDigit(cfg.Height, cfg.Width, cfg.Length, digitMaxSkew, digitDotCount)
	case DriverString:
		driver = base64Captcha.NewDriverString(
			cfg.Height, cfg.Width, cfg.NoiseCount, lineOptions(cfg.Lines), cfg.Length, cfg.Source, nil, nil, nil,
		)
	case DriverMath:
		driver = base64Captcha.NewDriverMath(
			cfg.Height, cfg.Width, cfg.NoiseCount, lineOptions(cfg.Lines), nil, nil, nil,
		)
	default:
		return nil, errors.Sentinel(ErrInvalidArgument)
	}

	if driver == nil {
		return nil, errors.Sentinel(ErrGenerateFailed)
	}
	return driver, nil
}

// lineOptions 将本包位掩码映射为上游位掩码
func lineOptions(lines LineOption) int {
	options := 0
	if lines&LineHollow != 0 {
		options |= base64Captcha.OptionShowHollowLine
	}
	if lines&LineSlime != 0 {
		options |= base64Captcha.OptionShowSlimeLine
	}
	if lines&LineSine != 0 {
		options |= base64Captcha.OptionShowSineLine
	}
	return options
}
