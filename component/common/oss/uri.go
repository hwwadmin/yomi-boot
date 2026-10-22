// Package oss 逻辑 URI（bucket/key）与固定访问 URL 拼装。
//
// @author zwei
package oss

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// JoinURI 将 bucket 与 key 拼为可入库的逻辑 URI（格式：bucket/key）。
// key 前导 "/" 会被去掉；bucket 不得包含 "/"。
func JoinURI(bucket, key string) (string, error) {
	bucket, key, err := normalizeBucketKey(bucket, key)
	if err != nil {
		return "", err
	}
	return bucket + "/" + key, nil
}

// ParseURI 解析逻辑 URI 为 bucket 与 key。
// 格式必须为 bucket/key；第一段为 bucket，其余为 key。
func ParseURI(uri string) (bucket, key string, err error) {
	uri = strings.TrimPrefix(strings.TrimSpace(uri), "/")
	if uri == "" {
		return "", "", errors.Wrap(ErrInvalidArgument, "oss: uri 不能为空")
	}
	bucket, key, ok := strings.Cut(uri, "/")
	if !ok || bucket == "" || key == "" {
		return "", "", errors.Wrap(ErrInvalidArgument, "oss: uri 格式无效，应为 bucket/key")
	}
	return bucket, key, nil
}

// normalizeBucketKey 规范化 bucket 和 key，去除首尾空格及 key 的前导 "/"，并校验非空
func normalizeBucketKey(bucket, key string) (string, string, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if bucket == "" {
		return "", "", errors.Wrap(ErrInvalidArgument, "oss: bucket 不能为空")
	}
	if key == "" {
		return "", "", errors.Wrap(ErrInvalidArgument, "oss: key 不能为空")
	}
	if strings.Contains(bucket, "/") {
		return "", "", errors.Wrap(ErrInvalidArgument, "oss: bucket 不能包含 /")
	}
	return bucket, key, nil
}

// buildObjectURL 将 public_base_url 与 uri 拼为固定访问 URL
func buildObjectURL(publicBaseURL, uri string) (string, error) {
	if _, _, err := ParseURI(uri); err != nil {
		return "", err
	}
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base == "" {
		return "", errors.Wrap(ErrInvalidArgument, "oss: public_base_url 未配置")
	}
	uri = strings.TrimPrefix(strings.TrimSpace(uri), "/")
	return base + "/" + uri, nil
}
