package filestorage

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// TestConfigValidate 验证 Config 构造时 provider 与必填项校验
func TestConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("unsupported provider", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{Provider: "oss"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("minio requires endpoint", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{
			Provider: ProviderMinIO,
			MinIO: MinIOConfig{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("s3 requires region", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{
			Provider: ProviderS3,
			S3: S3Config{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestNormalizeAndPresignArgs 验证 bucket/key 规范化与预签名参数校验
func TestNormalizeAndPresignArgs(t *testing.T) {
	t.Parallel()

	_, _, err := minioNormalizeBucketKey("", "k")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("bucket empty: got %v", err)
	}
	_, _, err = s3NormalizeBucketKey("b", "")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("key empty: got %v", err)
	}

	b, k, err := minioNormalizeBucketKey(" bucket ", "/a/b")
	if err != nil || b != "bucket" || k != "a/b" {
		t.Fatalf("minio normalize: bucket=%q key=%q err=%v", b, k, err)
	}
	b, k, err = s3NormalizeBucketKey(" bucket ", "/a/b")
	if err != nil || b != "bucket" || k != "a/b" {
		t.Fatalf("s3 normalize: bucket=%q key=%q err=%v", b, k, err)
	}

	store := &minioStorage{maxPresignExpiry: time.Hour}
	_, err = store.PresignGet(t.Context(), "b", "k", 0)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expiry<=0: got %v", err)
	}
	_, err = store.PresignGet(t.Context(), "b", "k", 2*time.Hour)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expiry too large: got %v", err)
	}
}
