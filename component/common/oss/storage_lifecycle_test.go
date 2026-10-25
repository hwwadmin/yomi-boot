// Storage 全生命周期集成测试：三个后端共享同一套断言，跑在内存 S3 fake 上
//
// @author zwei
package oss

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// 各后端预签名 URL 的签名参数名
const (
	awsPresignMarker    = "X-Amz-Signature"
	aliyunPresignMarker = "x-oss-signature"
)

// runStorageLifecycle 依次验证上传、下载、元数据、存在性、删除、固定 URL 与预签名
func runStorageLifecycle(t *testing.T, store Storage, missingBaseStore Storage, presignMarker string) {
	t.Helper()
	ctx := context.Background()
	const bucket, key, content = "bucket", "dir/file.txt", "hello-oss"

	// 上传：返回逻辑 URI，Content-Type 选项生效（由 fake 存储并在下载时回读验证）
	uri, err := store.PutObject(ctx, bucket, "/"+key, strings.NewReader(content), int64(len(content)), WithContentType("text/plain"))
	if err != nil {
		t.Fatalf("上传对象失败: %v", err)
	}
	if uri != bucket+"/"+key {
		t.Fatalf("上传应返回逻辑 URI bucket/dir/file.txt，实际 %q", uri)
	}

	// 上传参数校验
	if _, err := store.PutObject(ctx, "", key, strings.NewReader(content), 1); err == nil {
		t.Fatal("空 bucket 应返回错误")
	}
	if _, err := store.PutObject(ctx, bucket, "  ", strings.NewReader(content), 1); err == nil {
		t.Fatal("空 key 应返回错误")
	}
	if _, err := store.PutObject(ctx, bucket, key, nil, 1); err == nil {
		t.Fatal("nil body 应返回错误")
	}

	// 下载往返与元数据映射
	rc, meta, err := store.GetObject(ctx, bucket+"/"+key)
	if err != nil {
		t.Fatalf("下载对象失败: %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("读取对象内容失败: %v", err)
	}
	if string(got) != content {
		t.Fatalf("下载内容期望 %q，实际 %q", content, got)
	}
	if meta.ContentType != "text/plain" || meta.ContentLength != int64(len(content)) || meta.ETag == "" {
		t.Fatalf("元数据映射不符: %+v", meta)
	}

	// StatObject
	stat, err := store.StatObject(ctx, bucket+"/"+key)
	if err != nil {
		t.Fatalf("查询元数据失败: %v", err)
	}
	if stat.ContentLength != int64(len(content)) || stat.ETag == "" {
		t.Fatalf("StatObject 元数据不符: %+v", stat)
	}
	if _, err := store.StatObject(ctx, bucket+"/missing.txt"); err == nil {
		t.Fatal("查询不存在对象应返回错误")
	} else if err.Error() != ErrNotFound.Error() && !strings.Contains(err.Error(), "对象不存在") {
		t.Fatalf("不存在对象应映射 ErrNotFound，实际 %v", err)
	}

	// Exists
	if exists, err := store.Exists(ctx, bucket+"/"+key); err != nil || !exists {
		t.Fatalf("存在对象应返回 true: exists=%v err=%v", exists, err)
	}
	if exists, err := store.Exists(ctx, bucket+"/missing.txt"); err != nil || exists {
		t.Fatalf("不存在对象应返回 false,nil: exists=%v err=%v", exists, err)
	}
	if _, err := store.Exists(ctx, "bad-uri"); err == nil {
		t.Fatal("非法 URI 应返回错误")
	}

	// 非法 URI 的读删入口
	if _, _, err := store.GetObject(ctx, "bad-uri"); err == nil {
		t.Fatal("非法 URI 下载应返回错误")
	}
	if err := store.DeleteObject(ctx, "bad-uri"); err == nil {
		t.Fatal("非法 URI 删除应返回错误")
	}

	// 删除与幂等删除
	if err := store.DeleteObject(ctx, bucket+"/"+key); err != nil {
		t.Fatalf("删除对象失败: %v", err)
	}
	if err := store.DeleteObject(ctx, bucket+"/missing.txt"); err != nil {
		t.Fatalf("删除不存在对象应幂等返回 nil: %v", err)
	}
	if _, _, err := store.GetObject(ctx, bucket+"/"+key); err == nil {
		t.Fatal("删除后下载应返回错误")
	}

	// 固定访问 URL
	url, err := store.ObjectURL(bucket + "/" + key)
	if err != nil {
		t.Fatalf("拼装访问 URL 失败: %v", err)
	}
	if url != "https://cdn.example.com/"+bucket+"/"+key {
		t.Fatalf("访问 URL 不符: %q", url)
	}
	if _, err := missingBaseStore.ObjectURL(bucket + "/" + key); err == nil {
		t.Fatal("未配置 public_base_url 应返回错误")
	}

	// 预签名：成功 URL 携带签名参数，非法 expiry 被拒绝
	signed, err := store.PresignGet(ctx, bucket+"/"+key, time.Minute)
	if err != nil {
		t.Fatalf("生成预签名 URL 失败: %v", err)
	}
	if !strings.Contains(signed, presignMarker) {
		t.Fatalf("预签名 URL 应携带签名参数: %q", signed)
	}
	if _, err := store.PresignGet(ctx, bucket+"/"+key, 0); err == nil {
		t.Fatal("expiry 为 0 应返回错误")
	}
	if _, err := store.PresignGet(ctx, bucket+"/"+key, 8*24*time.Hour); err == nil {
		t.Fatal("expiry 超过上限应返回错误")
	}
	if _, err := store.PresignGet(ctx, "bad-uri", time.Minute); err == nil {
		t.Fatal("非法 URI 应返回错误")
	}
}

// runStorageErrorInjection 验证服务端故障时错误透传（非 404 不吞错）
func runStorageErrorInjection(t *testing.T, store Storage, fake *fakeS3) {
	t.Helper()
	ctx := context.Background()

	fake.fail = true
	defer func() { fake.fail = false }()

	if _, err := store.PutObject(ctx, "bucket", "err-key", strings.NewReader("v"), 1); err == nil {
		t.Fatal("服务端故障时上传应返回错误")
	}
	if err := store.DeleteObject(ctx, "bucket/err-key"); err == nil {
		t.Fatal("服务端故障时删除应返回错误")
	}
}

// TestRustFSStorageLifecycle RustFS 后端全生命周期
func TestRustFSStorageLifecycle(t *testing.T) {
	fake, endpoint := newFakeS3(t)
	runStorageLifecycle(t, newTestRustFSStorage(t, endpoint), newRustFSStorageWithoutBaseURL(t, endpoint), awsPresignMarker)
	runStorageErrorInjection(t, newTestRustFSStorage(t, endpoint), fake)
}

// TestS3StorageLifecycle AWS S3 后端全生命周期
func TestS3StorageLifecycle(t *testing.T) {
	fake, endpoint := newFakeS3(t)
	runStorageLifecycle(t, newTestS3Storage(t, endpoint), newS3StorageWithoutBaseURL(t, endpoint), awsPresignMarker)
	runStorageErrorInjection(t, newTestS3Storage(t, endpoint), fake)
}

// TestAliStorageLifecycle 阿里云 OSS 后端全生命周期
func TestAliStorageLifecycle(t *testing.T) {
	fake, endpoint := newFakeS3(t)
	runStorageLifecycle(t, newTestAliStorage(t, endpoint), newAliStorageWithoutBaseURL(t, endpoint), aliyunPresignMarker)
	runStorageErrorInjection(t, newTestAliStorage(t, endpoint), fake)
}

// newRustFSStorageWithoutBaseURL 构造未配置 PublicBaseURL 的 RustFS 存储
func newRustFSStorageWithoutBaseURL(t *testing.T, endpoint string) Storage {
	t.Helper()
	store, err := New(Config{
		Provider: ProviderRustFS,
		RustFS: RustFSConfig{
			Endpoint:        endpoint,
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
		},
	})
	if err != nil {
		t.Fatalf("创建 RustFS 存储失败: %v", err)
	}
	return store
}

// newS3StorageWithoutBaseURL 构造未配置 PublicBaseURL 的 S3 存储
func newS3StorageWithoutBaseURL(t *testing.T, endpoint string) Storage {
	t.Helper()
	store, err := New(Config{
		Provider: ProviderS3,
		S3: S3Config{
			Region:          "us-east-1",
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
			Endpoint:        endpoint,
			UsePathStyle:    true,
		},
	})
	if err != nil {
		t.Fatalf("创建 S3 存储失败: %v", err)
	}
	return store
}

// newAliStorageWithoutBaseURL 构造未配置 PublicBaseURL 的 AliOSS 存储
func newAliStorageWithoutBaseURL(t *testing.T, endpoint string) Storage {
	t.Helper()
	store, err := New(Config{
		Provider: ProviderAliOSS,
		AliOSS: AliOSSConfig{
			Region:          "cn-hangzhou",
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
			Endpoint:        endpoint,
			UsePathStyle:    true,
		},
	})
	if err != nil {
		t.Fatalf("创建 AliOSS 存储失败: %v", err)
	}
	return store
}
