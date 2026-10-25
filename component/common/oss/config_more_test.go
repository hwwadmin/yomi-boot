// 配置校验、可选参数与错误映射的单元测试
//
// @author zwei
package oss

import (
	"errors"
	"net/http"
	"testing"
	"time"

	aliyunoss "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aws/smithy-go"

	kerrors "gitee.com/hq_project/yomi-boot/component/core/errors"
)

// aliyunServiceErrorFixture 构造阿里云 SDK 服务错误
func aliyunServiceErrorFixture(statusCode int, code string) *aliyunoss.ServiceError {
	return &aliyunoss.ServiceError{
		Code:       code,
		Message:    "fixture",
		StatusCode: statusCode,
	}
}

// TestMaxPresignExpiryOrDefault 验证预签名上限默认值
func TestMaxPresignExpiryOrDefault(t *testing.T) {
	if got := (Config{}).maxPresignExpiryOrDefault(); got != 7*24*time.Hour {
		t.Fatalf("默认上限期望 7 天，实际 %v", got)
	}
	if got := (Config{MaxPresignExpiry: time.Hour}).maxPresignExpiryOrDefault(); got != time.Hour {
		t.Fatalf("自定义上限期望 1h，实际 %v", got)
	}
}

// TestSubConfigValidate 验证三个子配置的必填字段校验
func TestSubConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "rustfs 合法", cfg: Config{Provider: ProviderRustFS, RustFS: RustFSConfig{Endpoint: "e", AccessKeyID: "a", SecretAccessKey: "s"}}},
		{name: "rustfs 缺 endpoint", cfg: Config{Provider: ProviderRustFS, RustFS: RustFSConfig{AccessKeyID: "a", SecretAccessKey: "s"}}, wantErr: true},
		{name: "rustfs 缺 ak", cfg: Config{Provider: ProviderRustFS, RustFS: RustFSConfig{Endpoint: "e", SecretAccessKey: "s"}}, wantErr: true},
		{name: "rustfs 缺 sk", cfg: Config{Provider: ProviderRustFS, RustFS: RustFSConfig{Endpoint: "e", AccessKeyID: "a"}}, wantErr: true},
		{name: "s3 合法", cfg: Config{Provider: ProviderS3, S3: S3Config{Region: "r", AccessKeyID: "a", SecretAccessKey: "s"}}},
		{name: "s3 缺 region", cfg: Config{Provider: ProviderS3, S3: S3Config{AccessKeyID: "a", SecretAccessKey: "s"}}, wantErr: true},
		{name: "s3 缺 ak", cfg: Config{Provider: ProviderS3, S3: S3Config{Region: "r", SecretAccessKey: "s"}}, wantErr: true},
		{name: "s3 缺 sk", cfg: Config{Provider: ProviderS3, S3: S3Config{Region: "r", AccessKeyID: "a"}}, wantErr: true},
		{name: "alioss 合法", cfg: Config{Provider: ProviderAliOSS, AliOSS: AliOSSConfig{Region: "r", AccessKeyID: "a", SecretAccessKey: "s"}}},
		{name: "alioss 缺 region", cfg: Config{Provider: ProviderAliOSS, AliOSS: AliOSSConfig{AccessKeyID: "a", SecretAccessKey: "s"}}, wantErr: true},
		{name: "alioss 缺 ak", cfg: Config{Provider: ProviderAliOSS, AliOSS: AliOSSConfig{Region: "r", SecretAccessKey: "s"}}, wantErr: true},
		{name: "alioss 缺 sk", cfg: Config{Provider: ProviderAliOSS, AliOSS: AliOSSConfig{Region: "r", AccessKeyID: "a"}}, wantErr: true},
		{name: "provider 不支持", cfg: Config{Provider: "minio"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if tc.wantErr && err == nil {
				t.Fatalf("%s 应返回错误", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("%s 不应报错: %v", tc.name, err)
			}
		})
	}
}

// TestPutOptions 验证上传可选参数的叠加与 nil 容错
func TestPutOptions(t *testing.T) {
	opts := applyPutOptions([]PutOption{WithContentType("application/json"), nil})
	if opts.ContentType != "application/json" {
		t.Fatalf("ContentType 期望 application/json，实际 %q", opts.ContentType)
	}
	if got := applyPutOptions(nil); got.ContentType != "" {
		t.Fatalf("空选项应得零值，实际 %q", got.ContentType)
	}
}

// TestS3IsNotFound 验证 AWS 错误的不存在判定
func TestS3IsNotFound(t *testing.T) {
	if s3IsNotFound(nil) {
		t.Fatal("nil 错误不应判定为不存在")
	}
	if s3IsNotFound(errors.New("plain error")) {
		t.Fatal("普通错误不应判定为不存在")
	}
}

// TestS3NotFoundCodes 验证 smithy APIError 错误码判定
func TestS3NotFoundCodes(t *testing.T) {
	for _, code := range []string{"NoSuchKey", "NotFound", "NoSuchBucket"} {
		err := &smithy.GenericAPIError{Code: code, Message: "m"}
		if !s3IsNotFound(err) {
			t.Fatalf("错误码 %s 应判定为不存在", code)
		}
		if !rustfsIsNotFound(err) {
			t.Fatalf("RustFS 侧错误码 %s 应判定为不存在", code)
		}
	}
	other := &smithy.GenericAPIError{Code: "AccessDenied", Message: "m"}
	if s3IsNotFound(other) || rustfsIsNotFound(other) {
		t.Fatal("其他错误码不应判定为不存在")
	}
	if s3IsNotFound(errors.New("plain")) || rustfsIsNotFound(errors.New("plain")) {
		t.Fatal("普通错误不应判定为不存在")
	}
}

// TestMapErrorSentinels 验证三侧错误映射的哨兵转换
func TestMapErrorSentinels(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "NoSuchKey", Message: "m"}
	for name, mapped := range map[string]error{
		"s3":     s3MapError(notFound, "msg"),
		"rustfs": rustfsMapError(notFound, "msg"),
	} {
		if !kerrors.Is(mapped, ErrNotFound) {
			t.Fatalf("%s NotFound 应映射 ErrNotFound，实际 %v", name, mapped)
		}
	}
	if !kerrors.Is(aliOSSMapError(aliyunServiceErrorFixture(http.StatusNotFound, "NoSuchKey"), "msg"), ErrNotFound) {
		t.Fatal("AliOSS 404 应映射 ErrNotFound")
	}

	other := errors.New("boom")
	for name, mapped := range map[string]error{
		"s3":     s3MapError(other, "msg"),
		"rustfs": rustfsMapError(other, "msg"),
		"alioss": aliOSSMapError(other, "msg"),
	} {
		if kerrors.Is(mapped, ErrNotFound) {
			t.Fatalf("%s 普通错误不应映射 ErrNotFound，实际 %v", name, mapped)
		}
		if mapped == nil {
			t.Fatalf("%s 普通错误应被包装返回", name)
		}
	}
}

// TestAliOSSIsNotFound 验证阿里云错误的不存在判定
func TestAliOSSIsNotFound(t *testing.T) {
	if aliOSSIsNotFound(nil) {
		t.Fatal("nil 错误不应判定为不存在")
	}
	if !aliOSSIsNotFound(aliyunServiceErrorFixture(http.StatusNotFound, "")) {
		t.Fatal("StatusCode 404 应判定为不存在")
	}
	for _, code := range []string{"NoSuchKey", "NotFound", "NoSuchBucket"} {
		if !aliOSSIsNotFound(aliyunServiceErrorFixture(http.StatusForbidden, code)) {
			t.Fatalf("错误码 %s 应判定为不存在", code)
		}
	}
	if aliOSSIsNotFound(aliyunServiceErrorFixture(http.StatusForbidden, "AccessDenied")) {
		t.Fatal("其他错误不应判定为不存在")
	}
	if aliOSSIsNotFound(errors.New("plain")) {
		t.Fatal("普通错误不应判定为不存在")
	}
}

// TestAliOSSObjectMeta 验证阿里云响应字段的元数据映射
func TestAliOSSObjectMeta(t *testing.T) {
	now := time.Now()
	etag := `"etag"`
	ctype := "text/plain"
	meta := aliOSSObjectMeta(&ctype, 42, &etag, &now)
	if meta.ContentType != "text/plain" || meta.ContentLength != 42 || meta.ETag != `"etag"` || !meta.LastModified.Equal(now) {
		t.Fatalf("完整映射不符: %+v", meta)
	}

	empty := aliOSSObjectMeta(nil, -1, nil, nil)
	if empty.ContentType != "" || empty.ContentLength != 0 || empty.ETag != "" || !empty.LastModified.IsZero() {
		t.Fatalf("空字段应保持零值: %+v", empty)
	}
}

// TestMustNew 验证 Fail Fast 构造
func TestMustNew(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("非法配置应 panic")
		}
	}()
	MustNew(Config{Provider: "minio"})
}

// TestMustNewSuccess 验证合法配置的 Fail Fast 构造成功路径
func TestMustNewSuccess(t *testing.T) {
	_, endpoint := newFakeS3(t)
	store := MustNew(Config{
		Provider: ProviderRustFS,
		RustFS: RustFSConfig{
			Endpoint:        endpoint,
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
		},
	})
	if store == nil {
		t.Fatal("MustNew 应返回 Storage")
	}
}
