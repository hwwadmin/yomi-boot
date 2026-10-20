// Package oss 基于 alibabacloud-oss-go-sdk-v2 的 Storage 实现。
//
// @author zwei
package oss

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/errors"

	aliyunoss "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

var _ Storage = (*aliOSSStorage)(nil)

// aliOSSStorage 是基于阿里云 OSS 的 Storage 实现
type aliOSSStorage struct {
	client           *aliyunoss.Client
	publicBaseURL    string
	maxPresignExpiry time.Duration
}

// newAliOSSStorage 根据配置创建阿里云 OSS 存储实现
func newAliOSSStorage(cfg Config) (Storage, error) {
	region := strings.TrimSpace(cfg.AliOSS.Region)
	endpoint := strings.TrimSpace(cfg.AliOSS.Endpoint)

	ossCfg := aliyunoss.LoadDefaultConfig().
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AliOSS.AccessKeyID,
			cfg.AliOSS.SecretAccessKey,
		)).
		WithRegion(region)
	if endpoint != "" {
		ossCfg = ossCfg.WithEndpoint(endpoint)
	}
	if cfg.AliOSS.UsePathStyle {
		ossCfg = ossCfg.WithUsePathStyle(true)
	}

	return &aliOSSStorage{
		client:           aliyunoss.NewClient(ossCfg),
		publicBaseURL:    strings.TrimSpace(cfg.PublicBaseURL),
		maxPresignExpiry: cfg.maxPresignExpiryOrDefault(),
	}, nil
}

// PutObject 上传对象，成功返回可入库的逻辑 URI
func (s *aliOSSStorage) PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, opts ...PutOption) (string, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := normalizeBucketKey(bucket, key)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", errors.Wrap(ErrInvalidArgument, "oss: body 不能为 nil")
	}

	putOpts := applyPutOptions(opts)
	input := &aliyunoss.PutObjectRequest{
		Bucket: aliyunoss.Ptr(bucket),
		Key:    aliyunoss.Ptr(key),
		Body:   body,
	}
	if putOpts.ContentType != "" {
		input.ContentType = aliyunoss.Ptr(putOpts.ContentType)
	}
	if size >= 0 {
		input.ContentLength = aliyunoss.Ptr(size)
	}

	_, err = s.client.PutObject(ctx, input)
	if err != nil {
		return "", errors.Wrap(err, "oss: AliOSS 上传对象失败")
	}
	return JoinURI(bucket, key)
}

// GetObject 下载对象
func (s *aliOSSStorage) GetObject(ctx context.Context, uri string) (io.ReadCloser, *ObjectMeta, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := ParseURI(uri)
	if err != nil {
		return nil, nil, err
	}

	out, err := s.client.GetObject(ctx, &aliyunoss.GetObjectRequest{
		Bucket: aliyunoss.Ptr(bucket),
		Key:    aliyunoss.Ptr(key),
	})
	if err != nil {
		return nil, nil, aliOSSMapError(err, "oss: AliOSS 下载对象失败")
	}

	return out.Body, aliOSSObjectMeta(out.ContentType, out.ContentLength, out.ETag, out.LastModified), nil
}

// DeleteObject 删除对象；不存在时幂等返回 nil
func (s *aliOSSStorage) DeleteObject(ctx context.Context, uri string) error {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := ParseURI(uri)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteObject(ctx, &aliyunoss.DeleteObjectRequest{
		Bucket: aliyunoss.Ptr(bucket),
		Key:    aliyunoss.Ptr(key),
	})
	if err != nil {
		if aliOSSIsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "oss: AliOSS 删除对象失败")
	}
	return nil
}

// StatObject 查询对象元数据
func (s *aliOSSStorage) StatObject(ctx context.Context, uri string) (*ObjectMeta, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := ParseURI(uri)
	if err != nil {
		return nil, err
	}

	out, err := s.client.HeadObject(ctx, &aliyunoss.HeadObjectRequest{
		Bucket: aliyunoss.Ptr(bucket),
		Key:    aliyunoss.Ptr(key),
	})
	if err != nil {
		return nil, aliOSSMapError(err, "oss: AliOSS 查询对象失败")
	}

	return aliOSSObjectMeta(out.ContentType, out.ContentLength, out.ETag, out.LastModified), nil
}

// Exists 判断对象是否存在
func (s *aliOSSStorage) Exists(ctx context.Context, uri string) (bool, error) {
	_, err := s.StatObject(ctx, uri)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

// ObjectURL 返回对象的固定访问 URL
func (s *aliOSSStorage) ObjectURL(uri string) (string, error) {
	return buildObjectURL(s.publicBaseURL, uri)
}

// PresignGet 生成只读预签名 URL
func (s *aliOSSStorage) PresignGet(ctx context.Context, uri string, expiry time.Duration) (string, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := ParseURI(uri)
	if err != nil {
		return "", err
	}
	if expiry <= 0 {
		return "", errors.Wrap(ErrInvalidArgument, "oss: expiry 必须大于 0")
	}
	if expiry > s.maxPresignExpiry {
		return "", errors.Wrap(ErrInvalidArgument, "oss: expiry 超过允许上限")
	}

	out, err := s.client.Presign(ctx, &aliyunoss.GetObjectRequest{
		Bucket: aliyunoss.Ptr(bucket),
		Key:    aliyunoss.Ptr(key),
	}, aliyunoss.PresignExpires(expiry))
	if err != nil {
		return "", errors.Wrap(err, "oss: AliOSS 生成预签名 URL 失败")
	}
	return out.URL, nil
}

// aliOSSObjectMeta 从 OSS 响应字段映射 ObjectMeta
func aliOSSObjectMeta(contentType *string, contentLength int64, etag *string, lastModified *time.Time) *ObjectMeta {
	meta := &ObjectMeta{}
	if contentType != nil {
		meta.ContentType = *contentType
	}
	if contentLength >= 0 {
		meta.ContentLength = contentLength
	}
	if etag != nil {
		meta.ETag = *etag
	}
	if lastModified != nil {
		meta.LastModified = *lastModified
	}
	return meta
}

// aliOSSIsNotFound 判断阿里云 OSS 错误是否为对象不存在
func aliOSSIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var se *aliyunoss.ServiceError
	if errors.As(err, &se) {
		if se.StatusCode == http.StatusNotFound {
			return true
		}
		switch se.Code {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return true
		}
	}
	return false
}

// aliOSSMapError 将 AliOSS 错误映射为包内错误；NotFound 映射为 ErrNotFound，其余经 errors.Wrap 包装
func aliOSSMapError(err error, message string) error {
	if aliOSSIsNotFound(err) {
		return errors.Sentinel(ErrNotFound)
	}
	return errors.Wrap(err, "%s", message)
}
