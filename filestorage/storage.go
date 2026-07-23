// Package filestorage FileStorage 接口与公共类型。
//
// @author zwei
package filestorage

import (
	"context"
	"io"
	"time"
)

// FileStorage 统一文件/对象存储服务。
type FileStorage interface {
	// PutObject 上传对象。body 由调用方关闭；size < 0 表示长度未知（实现按 SDK 能力处理）。
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, opts ...PutOption) error

	// GetObject 下载对象。返回的 ReadCloser 必须由调用方 Close。
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, *ObjectMeta, error)

	// DeleteObject 删除对象。对象不存在时返回 nil（幂等删除）。
	DeleteObject(ctx context.Context, bucket, key string) error

	// StatObject 查询元数据。不存在返回 ErrNotFound。
	StatObject(ctx context.Context, bucket, key string) (*ObjectMeta, error)

	// Exists 判断对象是否存在。
	Exists(ctx context.Context, bucket, key string) (bool, error)

	// PresignGet 生成只读预签名 URL。expiry 必须 > 0，且不得超过 MaxPresignExpiry。
	// 不校验对象是否存在；对象缺失时错误发生在客户端使用该 URL 拉取时。
	PresignGet(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// ObjectMeta 对象元数据（实现从 SDK 映射，字段可为空值）。
type ObjectMeta struct {
	ContentType   string    // ContentType 是对象 MIME 类型
	ContentLength int64     // ContentLength 是对象字节长度
	ETag          string    // ETag 是对象实体标签
	LastModified  time.Time // LastModified 是最后修改时间
}
