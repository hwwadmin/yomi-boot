// Package filestorage 基于 minio-go 的 FileStorage 实现。
//
// @author zwei
package filestorage

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
	"yt-boot/appctx"
	"yt-boot/errors"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var _ FileStorage = (*minioStorage)(nil)

const minioDefaultRegion = "us-east-1"

// minioStorage 是基于 MinIO 的 FileStorage 实现
type minioStorage struct {
	client           *minio.Client
	maxPresignExpiry time.Duration
}

// newMinIOStorage 根据配置创建 MinIO 存储实现
func newMinIOStorage(cfg Config) (FileStorage, error) {
	endpoint := strings.TrimSpace(cfg.MinIO.Endpoint)
	region := strings.TrimSpace(cfg.MinIO.Region)
	if region == "" {
		region = minioDefaultRegion
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIO.AccessKeyID, cfg.MinIO.SecretAccessKey, ""),
		Secure: cfg.MinIO.UseSSL,
		Region: region,
	})
	if err != nil {
		return nil, errors.Wrap(err, "filestorage: 创建 MinIO 客户端失败")
	}

	return &minioStorage{
		client:           client,
		maxPresignExpiry: cfg.maxPresignExpiryOrDefault(),
	}, nil
}

// PutObject 上传对象
func (s *minioStorage) PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, opts ...PutOption) error {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := minioNormalizeBucketKey(bucket, key)
	if err != nil {
		return err
	}
	if body == nil {
		return errors.Wrap(ErrInvalidArgument, "filestorage: body 不能为 nil")
	}

	putOpts := applyPutOptions(opts)
	objectOpts := minio.PutObjectOptions{}
	if putOpts.ContentType != "" {
		objectOpts.ContentType = putOpts.ContentType
	}

	_, err = s.client.PutObject(ctx, bucket, key, body, size, objectOpts)
	if err != nil {
		return errors.Wrap(err, "filestorage: MinIO 上传对象失败")
	}
	return nil
}

// GetObject 下载对象
func (s *minioStorage) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, *ObjectMeta, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := minioNormalizeBucketKey(bucket, key)
	if err != nil {
		return nil, nil, err
	}

	obj, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, minioMapError(err, "filestorage: MinIO 下载对象失败")
	}

	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, nil, minioMapError(err, "filestorage: MinIO 下载对象失败")
	}

	return obj, &ObjectMeta{
		ContentType:   info.ContentType,
		ContentLength: info.Size,
		ETag:          info.ETag,
		LastModified:  info.LastModified,
	}, nil
}

// DeleteObject 删除对象；不存在时幂等返回 nil
func (s *minioStorage) DeleteObject(ctx context.Context, bucket, key string) error {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := minioNormalizeBucketKey(bucket, key)
	if err != nil {
		return err
	}

	err = s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		if minioIsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "filestorage: MinIO 删除对象失败")
	}
	return nil
}

// StatObject 查询对象元数据
func (s *minioStorage) StatObject(ctx context.Context, bucket, key string) (*ObjectMeta, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := minioNormalizeBucketKey(bucket, key)
	if err != nil {
		return nil, err
	}

	info, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, minioMapError(err, "filestorage: MinIO 查询对象失败")
	}

	return &ObjectMeta{
		ContentType:   info.ContentType,
		ContentLength: info.Size,
		ETag:          info.ETag,
		LastModified:  info.LastModified,
	}, nil
}

// Exists 判断对象是否存在
func (s *minioStorage) Exists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := s.StatObject(ctx, bucket, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

// PresignGet 生成只读预签名 URL
func (s *minioStorage) PresignGet(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := minioNormalizeBucketKey(bucket, key)
	if err != nil {
		return "", err
	}
	if expiry <= 0 {
		return "", errors.Wrap(ErrInvalidArgument, "filestorage: expiry 必须大于 0")
	}
	if expiry > s.maxPresignExpiry {
		return "", errors.Wrap(ErrInvalidArgument, "filestorage: expiry 超过允许上限")
	}

	u, err := s.client.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		return "", errors.Wrap(err, "filestorage: MinIO 生成预签名 URL 失败")
	}
	return u.String(), nil
}

// minioNormalizeBucketKey 规范化 bucket 和 key，去除首尾空格及 key 的前导 "/"，并校验非空
func minioNormalizeBucketKey(bucket, key string) (string, string, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if bucket == "" {
		return "", "", errors.Wrap(ErrInvalidArgument, "filestorage: bucket 不能为空")
	}
	if key == "" {
		return "", "", errors.Wrap(ErrInvalidArgument, "filestorage: key 不能为空")
	}
	return bucket, key, nil
}

// minioIsNotFound 判断 MinIO 错误是否为对象不存在
func minioIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	if resp.StatusCode == http.StatusNotFound {
		return true
	}
	switch resp.Code {
	case "NoSuchKey", "NoSuchObject", "NotFound", "ResourceNotFound":
		return true
	default:
		return false
	}
}

// minioMapError 将 MinIO 错误映射为包内错误；NotFound 映射为 ErrNotFound，其余经 errors.Wrap 包装
func minioMapError(err error, message string) error {
	if minioIsNotFound(err) {
		return errors.Sentinel(ErrNotFound)
	}
	return errors.Wrap(err, "%s", message)
}
