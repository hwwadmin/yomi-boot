// Package filestorage New 构造函数：按 provider 分发实现。
//
// @author zwei
package filestorage

import "yt-boot/errors"

// New 根据配置创建 FileStorage
func New(cfg Config) (FileStorage, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	switch cfg.Provider {
	case ProviderMinIO:
		return newMinIOStorage(cfg)
	case ProviderS3:
		return newS3Storage(cfg)
	default:
		// validate 已覆盖，此处为防御
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
}
