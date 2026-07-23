// Package filestorage 定义对象存储相关的 sentinel 错误。
//
// @author zwei
package filestorage

import "yt-boot/errors"

var (
	// ErrNotFound 对象不存在（GetObject / StatObject）
	ErrNotFound = errors.NewNoStack("filestorage: 对象不存在")

	// ErrInvalidArgument 参数无效（空 bucket/key、expiry 非法等）
	ErrInvalidArgument = errors.NewNoStack("filestorage: 参数无效")
)
