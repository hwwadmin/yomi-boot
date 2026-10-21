// Package oss PutObject 可选参数。
//
// @author zwei
package oss

// PutOptions 上传可选参数
type PutOptions struct {
	ContentType string // ContentType 是对象 Content-Type；空表示由存储端默认
}

// PutOption 自定义上传参数
type PutOption func(*PutOptions)

// WithContentType 设置对象 Content-Type
func WithContentType(contentType string) PutOption {
	return func(o *PutOptions) {
		o.ContentType = contentType
	}
}

// applyPutOptions 应用 PutOption 列表到 PutOptions，忽略 nil 函数
func applyPutOptions(opts []PutOption) PutOptions {
	var o PutOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}
