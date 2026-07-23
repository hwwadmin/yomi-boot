// Package filestorage 提供统一的文件/对象存储服务，支持 MinIO 与 AWS S3。
//
// # 基本用法
//
//	store, err := filestorage.New(filestorage.Config{
//	    Provider: filestorage.ProviderMinIO,
//	    MinIO: filestorage.MinIOConfig{
//	        Endpoint:        "127.0.0.1:9000",
//	        AccessKeyID:     "...",
//	        SecretAccessKey: "...",
//	    },
//	})
//	if err != nil {
//	    return err
//	}
//
//	err = store.PutObject(ctx, "avatars", "u/1.png", reader, size, filestorage.WithContentType("image/png"))
//	url, err := store.PresignGet(ctx, "avatars", "u/1.png", 15*time.Minute)
//
// # 语义约定
//
//   - GetObject / StatObject：对象不存在返回 ErrNotFound
//   - DeleteObject：对象不存在时幂等返回 nil
//   - PresignGet：仅支持只读预签名；不校验对象是否存在；expiry 必须 > 0 且不超过 MaxPresignExpiry（默认 7 天）
//   - key 前导 "/" 会被去掉
//
// # 实现隔离
//
// MinIO（minio.go）与 AWS S3（s3.go）完全独立实现，禁止共享业务逻辑。
//
// @author zwei
package filestorage
