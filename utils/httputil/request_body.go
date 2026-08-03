// Package httputil 标准库 HTTP 请求相关的工具函数
//
// @author zwei
package httputil

import (
	"bytes"
	"io"
	"net/http"
)

// multiReadCloser 将 Reader 与 Closer 组合，Close 委托给底层 Closer
type multiReadCloser struct {
	io.Reader
	io.Closer
}

// ReplayRequestBody 在已对 original 做有限读取后，为下游还原可再次读取的 Body。
// recorded 为已读出的字节（通常为 LimitReader(maxSize+1) 的结果）：
//   - len(recorded) <= maxSize：original 已读至 EOF，关闭后用内存 Reader 替换
//   - len(recorded) > maxSize：用 MultiReader 拼接 recorded 与剩余 original，Close 委托给 original
//
// 避免使用 io.NopCloser 包装仍指向原始 *http.body 的 MultiReader，以免 Close 语义丢失。
func ReplayRequestBody(r *http.Request, original io.ReadCloser, recorded []byte, maxSize int) {
	if r == nil {
		return
	}
	if original == nil {
		r.Body = http.NoBody
		return
	}
	if maxSize < 0 {
		maxSize = 0
	}
	if len(recorded) <= maxSize {
		_ = original.Close()
		if len(recorded) == 0 {
			r.Body = http.NoBody
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(recorded))
		return
	}
	r.Body = &multiReadCloser{
		Reader: io.MultiReader(bytes.NewReader(recorded), original),
		Closer: original,
	}
}
