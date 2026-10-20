// Package oss New 构造函数：按 provider 分发实现。
//
// @author zwei
package oss

import "gitee.com/hq_project/yomi-boot/errors"

// New 根据配置创建 Storage
func New(cfg Config) (Storage, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	switch cfg.Provider {
	case ProviderRustFS:
		return newRustFSStorage(cfg)
	case ProviderS3:
		return newS3Storage(cfg)
	case ProviderAliOSS:
		return newAliOSSStorage(cfg)
	default:
		// validate 已覆盖，此处为防御
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
}
