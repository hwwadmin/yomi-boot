// Package oss 提供对象存储启动配置（YAML mapstructure）
//
// @author zwei
package oss

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/errors"
)

const (
	// SectionKey YAML 根 key
	SectionKey = "oss"

	bootProviderMinIO  = "minio"
	bootProviderS3     = "s3"
	bootProviderAliOSS = "alioss"
)

// BootConfig 表示对象存储访问配置（启动期 YAML 绑定）
type BootConfig struct {
	Enabled                 *bool            `mapstructure:"enabled"`                    // Enabled 是否启用；未配置默认 false
	Provider                string           `mapstructure:"provider"`                   // Provider 是 minio、s3 或 alioss
	PublicBaseURL           string           `mapstructure:"public_base_url"`            // PublicBaseURL 是固定访问 URL 的全局基址（多桶共用）
	MaxPresignExpirySeconds int              `mapstructure:"max_presign_expiry_seconds"` // MaxPresignExpirySeconds 预签名有效期上限（秒）；0 表示由实现默认
	MinIO                   BootMinIOConfig  `mapstructure:"minio"`                      // MinIO 是 MinIO 访问配置
	S3                      BootS3Config     `mapstructure:"s3"`                         // S3 是 AWS S3 访问配置
	AliOSS                  BootAliOSSConfig `mapstructure:"alioss"`                     // AliOSS 是阿里云 OSS 访问配置
}

// BootMinIOConfig MinIO 访问配置
type BootMinIOConfig struct {
	Endpoint        string `mapstructure:"endpoint"`          // Endpoint 是 MinIO 地址（不含 scheme）
	AccessKeyID     string `mapstructure:"access_key_id"`     // AccessKeyID 是访问密钥 ID
	SecretAccessKey string `mapstructure:"secret_access_key"` // SecretAccessKey 是访问密钥
	UseSSL          bool   `mapstructure:"use_ssl"`           // UseSSL 是否使用 HTTPS
	Region          string `mapstructure:"region"`            // Region 是区域
}

// BootS3Config AWS S3 访问配置
type BootS3Config struct {
	Region          string `mapstructure:"region"`            // Region 是 AWS 区域
	AccessKeyID     string `mapstructure:"access_key_id"`     // AccessKeyID 是访问密钥 ID
	SecretAccessKey string `mapstructure:"secret_access_key"` // SecretAccessKey 是访问密钥
	Endpoint        string `mapstructure:"endpoint"`          // Endpoint 是自定义端点；空表示官方 AWS
	UsePathStyle    bool   `mapstructure:"use_path_style"`    // UsePathStyle 是否使用 path-style 寻址
}

// BootAliOSSConfig 阿里云 OSS 访问配置
type BootAliOSSConfig struct {
	Region          string `mapstructure:"region"`            // Region 是 OSS 区域，如 cn-hangzhou
	AccessKeyID     string `mapstructure:"access_key_id"`     // AccessKeyID 是访问密钥 ID
	SecretAccessKey string `mapstructure:"secret_access_key"` // SecretAccessKey 是访问密钥
	Endpoint        string `mapstructure:"endpoint"`          // Endpoint 是自定义端点；空表示按 Region 使用官方域名
	UsePathStyle    bool   `mapstructure:"use_path_style"`    // UsePathStyle 是否使用 path-style 寻址
}

// EnabledOrDefault 返回是否启用对象存储，未配置默认 false
func (c BootConfig) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// Validate 校验对象存储配置语义（仅 enabled=true 时校验）
func (c BootConfig) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	if c.MaxPresignExpirySeconds < 0 {
		return errors.New("oss.max_presign_expiry_seconds 不能小于 0")
	}
	provider := strings.ToLower(strings.TrimSpace(c.Provider))
	switch provider {
	case bootProviderMinIO:
		return c.MinIO.validate()
	case bootProviderS3:
		return c.S3.validate()
	case bootProviderAliOSS:
		return c.AliOSS.validate()
	case "":
		return errors.New("oss.provider 不能为空")
	default:
		return errors.Newf("oss.provider 不支持: %s", c.Provider)
	}
}

// validate 校验 MinIO 访问配置
func (c BootMinIOConfig) validate() error {
	if strings.TrimSpace(c.Endpoint) == "" {
		return errors.New("oss.minio.endpoint 不能为空")
	}
	if strings.TrimSpace(c.AccessKeyID) == "" {
		return errors.New("oss.minio.access_key_id 不能为空")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		return errors.New("oss.minio.secret_access_key 不能为空")
	}
	return nil
}

// validate 校验 S3 访问配置
func (c BootS3Config) validate() error {
	if strings.TrimSpace(c.Region) == "" {
		return errors.New("oss.s3.region 不能为空")
	}
	if strings.TrimSpace(c.AccessKeyID) == "" {
		return errors.New("oss.s3.access_key_id 不能为空")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		return errors.New("oss.s3.secret_access_key 不能为空")
	}
	return nil
}

// validate 校验 AliOSS 访问配置
func (c BootAliOSSConfig) validate() error {
	if strings.TrimSpace(c.Region) == "" {
		return errors.New("oss.alioss.region 不能为空")
	}
	if strings.TrimSpace(c.AccessKeyID) == "" {
		return errors.New("oss.alioss.access_key_id 不能为空")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		return errors.New("oss.alioss.secret_access_key 不能为空")
	}
	return nil
}

// ConfigSection 返回 oss 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Document) error {
		return c.Validate()
	})
}

// ConfigFrom 将启动配置转换为包内构造配置；调用方须先保证 cfg 已通过 Validate。
func ConfigFrom(cfg BootConfig) Config {
	provider := Provider(strings.ToLower(strings.TrimSpace(cfg.Provider)))
	out := Config{
		Provider:      provider,
		PublicBaseURL: strings.TrimSpace(cfg.PublicBaseURL),
		MinIO: MinIOConfig{
			Endpoint:        cfg.MinIO.Endpoint,
			AccessKeyID:     cfg.MinIO.AccessKeyID,
			SecretAccessKey: cfg.MinIO.SecretAccessKey,
			UseSSL:          cfg.MinIO.UseSSL,
			Region:          cfg.MinIO.Region,
		},
		S3: S3Config{
			Region:          cfg.S3.Region,
			AccessKeyID:     cfg.S3.AccessKeyID,
			SecretAccessKey: cfg.S3.SecretAccessKey,
			Endpoint:        cfg.S3.Endpoint,
			UsePathStyle:    cfg.S3.UsePathStyle,
		},
		AliOSS: AliOSSConfig{
			Region:          cfg.AliOSS.Region,
			AccessKeyID:     cfg.AliOSS.AccessKeyID,
			SecretAccessKey: cfg.AliOSS.SecretAccessKey,
			Endpoint:        cfg.AliOSS.Endpoint,
			UsePathStyle:    cfg.AliOSS.UsePathStyle,
		},
	}
	if cfg.MaxPresignExpirySeconds > 0 {
		out.MaxPresignExpiry = time.Duration(cfg.MaxPresignExpirySeconds) * time.Second
	}
	return out
}
