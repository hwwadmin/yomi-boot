// Package oss 基于 aws-sdk-go-v2 的 RustFS Storage 实现。
//
// @author zwei
package oss

import (
	"context"
	"io"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/core/appctx"
	"gitee.com/hq_project/yomi-boot/core/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

var _ Storage = (*rustfsStorage)(nil)

const rustfsDefaultRegion = "us-east-1"

// rustfsStorage 是基于 RustFS 的 Storage 实现
type rustfsStorage struct {
	client           *s3.Client
	presigner        *s3.PresignClient
	publicBaseURL    string
	maxPresignExpiry time.Duration
}

// newRustFSStorage 根据配置创建 RustFS 存储实现
func newRustFSStorage(cfg Config) (Storage, error) {
	region := strings.TrimSpace(cfg.RustFS.Region)
	if region == "" {
		region = rustfsDefaultRegion
	}
	endpoint := rustFSEndpoint(cfg.RustFS.Endpoint, cfg.RustFS.UseSSL)

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.RustFS.AccessKeyID,
			cfg.RustFS.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, errors.Wrap(err, "oss: 加载 RustFS 配置失败")
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return &rustfsStorage{
		client:           client,
		presigner:        s3.NewPresignClient(client),
		publicBaseURL:    strings.TrimSpace(cfg.PublicBaseURL),
		maxPresignExpiry: cfg.maxPresignExpiryOrDefault(),
	}, nil
}

// rustFSEndpoint 归一化 RustFS 端点：未带 scheme 时按 useSSL 补 http/https
func rustFSEndpoint(endpoint string, useSSL bool) string {
	endpoint = strings.TrimSpace(endpoint)
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	if useSSL {
		return "https://" + endpoint
	}
	return "http://" + endpoint
}

// PutObject 上传对象，成功返回可入库的逻辑 URI
func (s *rustfsStorage) PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, opts ...PutOption) (string, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := normalizeBucketKey(bucket, key)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", errors.Wrap(ErrInvalidArgument, "oss: body 不能为 nil")
	}

	putOpts := applyPutOptions(opts)
	input := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if putOpts.ContentType != "" {
		input.ContentType = aws.String(putOpts.ContentType)
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}

	_, err = s.client.PutObject(ctx, input)
	if err != nil {
		return "", errors.Wrap(err, "oss: RustFS 上传对象失败")
	}
	return JoinURI(bucket, key)
}

// GetObject 下载对象
func (s *rustfsStorage) GetObject(ctx context.Context, uri string) (io.ReadCloser, *ObjectMeta, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := ParseURI(uri)
	if err != nil {
		return nil, nil, err
	}

	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, nil, rustfsMapError(err, "oss: RustFS 下载对象失败")
	}

	meta := &ObjectMeta{}
	if out.ContentType != nil {
		meta.ContentType = *out.ContentType
	}
	if out.ContentLength != nil {
		meta.ContentLength = *out.ContentLength
	}
	if out.ETag != nil {
		meta.ETag = *out.ETag
	}
	if out.LastModified != nil {
		meta.LastModified = *out.LastModified
	}

	return out.Body, meta, nil
}

// DeleteObject 删除对象；不存在时幂等返回 nil
func (s *rustfsStorage) DeleteObject(ctx context.Context, uri string) error {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := ParseURI(uri)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if rustfsIsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "oss: RustFS 删除对象失败")
	}
	return nil
}

// StatObject 查询对象元数据
func (s *rustfsStorage) StatObject(ctx context.Context, uri string) (*ObjectMeta, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := ParseURI(uri)
	if err != nil {
		return nil, err
	}

	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, rustfsMapError(err, "oss: RustFS 查询对象失败")
	}

	meta := &ObjectMeta{}
	if out.ContentType != nil {
		meta.ContentType = *out.ContentType
	}
	if out.ContentLength != nil {
		meta.ContentLength = *out.ContentLength
	}
	if out.ETag != nil {
		meta.ETag = *out.ETag
	}
	if out.LastModified != nil {
		meta.LastModified = *out.LastModified
	}
	return meta, nil
}

// Exists 判断对象是否存在
func (s *rustfsStorage) Exists(ctx context.Context, uri string) (bool, error) {
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
func (s *rustfsStorage) ObjectURL(uri string) (string, error) {
	return buildObjectURL(s.publicBaseURL, uri)
}

// PresignGet 生成只读预签名 URL
func (s *rustfsStorage) PresignGet(ctx context.Context, uri string, expiry time.Duration) (string, error) {
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

	out, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", errors.Wrap(err, "oss: RustFS 生成预签名 URL 失败")
	}
	return out.URL, nil
}

// rustfsIsNotFound 判断 RustFS 错误是否为对象不存在
func rustfsIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return true
		}
	}
	return false
}

// rustfsMapError 将 RustFS 错误映射为包内错误；NotFound 映射为 ErrNotFound，其余经 errors.Wrap 包装
func rustfsMapError(err error, message string) error {
	if rustfsIsNotFound(err) {
		return errors.Sentinel(ErrNotFound)
	}
	return errors.Wrap(err, "%s", message)
}
