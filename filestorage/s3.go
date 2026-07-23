// Package filestorage 基于 aws-sdk-go-v2 的 FileStorage 实现。
//
// @author zwei
package filestorage

import (
	"context"
	"io"
	"strings"
	"time"
	"yomi-boot/appctx"
	"yomi-boot/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

var _ FileStorage = (*s3Storage)(nil)

// s3Storage 是基于 AWS S3 的 FileStorage 实现
type s3Storage struct {
	client           *s3.Client
	presigner        *s3.PresignClient
	maxPresignExpiry time.Duration
}

// newS3Storage 根据配置创建 AWS S3 存储实现
func newS3Storage(cfg Config) (FileStorage, error) {
	region := strings.TrimSpace(cfg.S3.Region)
	endpoint := strings.TrimSpace(cfg.S3.Endpoint)

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3.AccessKeyID,
			cfg.S3.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, errors.Wrap(err, "filestorage: 加载 AWS 配置失败")
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = cfg.S3.UsePathStyle
		}
	})

	return &s3Storage{
		client:           client,
		presigner:        s3.NewPresignClient(client),
		maxPresignExpiry: cfg.maxPresignExpiryOrDefault(),
	}, nil
}

// PutObject 上传对象
func (s *s3Storage) PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, opts ...PutOption) error {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := s3NormalizeBucketKey(bucket, key)
	if err != nil {
		return err
	}
	if body == nil {
		return errors.Wrap(ErrInvalidArgument, "filestorage: body 不能为 nil")
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
		return errors.Wrap(err, "filestorage: S3 上传对象失败")
	}
	return nil
}

// GetObject 下载对象
func (s *s3Storage) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, *ObjectMeta, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := s3NormalizeBucketKey(bucket, key)
	if err != nil {
		return nil, nil, err
	}

	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, nil, s3MapError(err, "filestorage: S3 下载对象失败")
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
func (s *s3Storage) DeleteObject(ctx context.Context, bucket, key string) error {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := s3NormalizeBucketKey(bucket, key)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if s3IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "filestorage: S3 删除对象失败")
	}
	return nil
}

// StatObject 查询对象元数据
func (s *s3Storage) StatObject(ctx context.Context, bucket, key string) (*ObjectMeta, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := s3NormalizeBucketKey(bucket, key)
	if err != nil {
		return nil, err
	}

	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, s3MapError(err, "filestorage: S3 查询对象失败")
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
func (s *s3Storage) Exists(ctx context.Context, bucket, key string) (bool, error) {
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
func (s *s3Storage) PresignGet(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	ctx = appctx.OrBackground(ctx)
	bucket, key, err := s3NormalizeBucketKey(bucket, key)
	if err != nil {
		return "", err
	}
	if expiry <= 0 {
		return "", errors.Wrap(ErrInvalidArgument, "filestorage: expiry 必须大于 0")
	}
	if expiry > s.maxPresignExpiry {
		return "", errors.Wrap(ErrInvalidArgument, "filestorage: expiry 超过允许上限")
	}

	out, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", errors.Wrap(err, "filestorage: S3 生成预签名 URL 失败")
	}
	return out.URL, nil
}

// s3NormalizeBucketKey 规范化 bucket 和 key，去除首尾空格及 key 的前导 "/"，并校验非空
func s3NormalizeBucketKey(bucket, key string) (string, string, error) {
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

// s3IsNotFound 判断 AWS S3 错误是否为对象不存在
func s3IsNotFound(err error) bool {
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

// s3MapError 将 S3 错误映射为包内错误；NotFound 映射为 ErrNotFound，其余经 errors.Wrap 包装
func s3MapError(err error, message string) error {
	if s3IsNotFound(err) {
		return errors.Sentinel(ErrNotFound)
	}
	return errors.Wrap(err, "%s", message)
}
