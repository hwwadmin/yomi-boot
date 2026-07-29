// Package filestorage 构造配置（与 mapstructure YAML 配置分离）。
//
// @author zwei
package filestorage

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Provider 对象存储提供者标识
type Provider string

const (
	// ProviderMinIO 使用 MinIO
	ProviderMinIO Provider = "minio"
	// ProviderS3 使用 AWS S3
	ProviderS3 Provider = "s3"
	// ProviderAliOSS 使用阿里云 OSS
	ProviderAliOSS Provider = "alioss"

	defaultMaxPresignExpiry = 7 * 24 * time.Hour
)

// Config 创建 FileStorage 的构造配置
type Config struct {
	Provider         Provider      // Provider 是存储实现标识
	MaxPresignExpiry time.Duration // MaxPresignExpiry 是预签名有效期上限；0 表示使用默认 7 天
	MinIO            MinIOConfig   // MinIO 是 MinIO 访问配置（Provider=minio 时必填）
	S3               S3Config      // S3 是 AWS S3 访问配置（Provider=s3 时必填）
	AliOSS           AliOSSConfig  // AliOSS 是阿里云 OSS 访问配置（Provider=alioss 时必填）
}

// MinIOConfig MinIO 访问配置
type MinIOConfig struct {
	Endpoint        string // Endpoint 是 MinIO 地址，如 127.0.0.1:9000（不含 scheme）
	AccessKeyID     string // AccessKeyID 是访问密钥 ID
	SecretAccessKey string // SecretAccessKey 是访问密钥
	UseSSL          bool   // UseSSL 是否使用 HTTPS
	Region          string // Region 是区域；空时使用 us-east-1
}

// S3Config AWS S3 访问配置
type S3Config struct {
	Region          string // Region 是 AWS 区域
	AccessKeyID     string // AccessKeyID 是访问密钥 ID
	SecretAccessKey string // SecretAccessKey 是访问密钥
	Endpoint        string // Endpoint 是自定义端点；空表示官方 AWS
	UsePathStyle    bool   // UsePathStyle 是否使用 path-style 寻址
}

// AliOSSConfig 阿里云 OSS 访问配置
type AliOSSConfig struct {
	Region          string // Region 是 OSS 区域，如 cn-hangzhou
	AccessKeyID     string // AccessKeyID 是访问密钥 ID
	SecretAccessKey string // SecretAccessKey 是访问密钥
	Endpoint        string // Endpoint 是自定义端点；空表示按 Region 使用官方域名
	UsePathStyle    bool   // UsePathStyle 是否使用 path-style 寻址
}

// maxPresignExpiryOrDefault 返回预签名有效期上限，0 表示使用默认 7 天
func (c Config) maxPresignExpiryOrDefault() time.Duration {
	if c.MaxPresignExpiry > 0 {
		return c.MaxPresignExpiry
	}
	return defaultMaxPresignExpiry
}

// validate 校验配置有效性，按 Provider 分发到对应子配置的校验逻辑
func (c Config) validate() error {
	switch c.Provider {
	case ProviderMinIO:
		return c.MinIO.validate()
	case ProviderS3:
		return c.S3.validate()
	case ProviderAliOSS:
		return c.AliOSS.validate()
	default:
		return errors.Newf("filestorage: 不支持的 provider: %s", c.Provider)
	}
}

// validate 校验 MinIO 配置必填字段（Endpoint / AccessKeyID / SecretAccessKey）
func (c MinIOConfig) validate() error {
	if strings.TrimSpace(c.Endpoint) == "" {
		return errors.New("filestorage: minio.endpoint 不能为空")
	}
	if strings.TrimSpace(c.AccessKeyID) == "" {
		return errors.New("filestorage: minio.access_key_id 不能为空")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		return errors.New("filestorage: minio.secret_access_key 不能为空")
	}
	return nil
}

// validate 校验 S3 配置必填字段（Region / AccessKeyID / SecretAccessKey）
func (c S3Config) validate() error {
	if strings.TrimSpace(c.Region) == "" {
		return errors.New("filestorage: s3.region 不能为空")
	}
	if strings.TrimSpace(c.AccessKeyID) == "" {
		return errors.New("filestorage: s3.access_key_id 不能为空")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		return errors.New("filestorage: s3.secret_access_key 不能为空")
	}
	return nil
}

// validate 校验 AliOSS 配置必填字段（Region / AccessKeyID / SecretAccessKey）
func (c AliOSSConfig) validate() error {
	if strings.TrimSpace(c.Region) == "" {
		return errors.New("filestorage: alioss.region 不能为空")
	}
	if strings.TrimSpace(c.AccessKeyID) == "" {
		return errors.New("filestorage: alioss.access_key_id 不能为空")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		return errors.New("filestorage: alioss.secret_access_key 不能为空")
	}
	return nil
}
