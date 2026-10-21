// Package oss 定义对象存储相关的 sentinel 错误。
//
// @author zwei
package oss

import "gitee.com/hq_project/yomi-boot/boot/errors"

var (
	// ErrNotFound 对象不存在（GetObject / StatObject）
	ErrNotFound = errors.NewNoStack("oss: 对象不存在")

	// ErrInvalidArgument 参数无效（空 bucket/key/uri、expiry 非法、public_base_url 未配置等）
	ErrInvalidArgument = errors.NewNoStack("oss: 参数无效")
)
