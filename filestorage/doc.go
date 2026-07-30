// Package filestorage 提供统一的文件/对象存储服务，支持 MinIO、AWS S3 与阿里云 OSS。
//
// # 基本用法
//
//	store, err := filestorage.New(filestorage.Config{
//	    Provider:      filestorage.ProviderMinIO,
//	    PublicBaseURL: "https://cdn.example.com",
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
//	uri, err := store.PutObject(ctx, "avatars", "u/1.png", reader, size, filestorage.WithContentType("image/png"))
//	// 入库 uri，例如 "avatars/u/1.png"；不要存完整 HTTP URL
//	url, err := store.ObjectURL(uri)                                 // 固定访问 URL
//	url, err = store.PresignGet(ctx, uri, 15*time.Minute)            // 临时预签名 URL
//
// # 概念约定
//
//   - key：桶内对象路径（如 u/1.png）
//   - uri：逻辑定位符 bucket/key（如 avatars/u/1.png），适合持久化
//   - url：完整 HTTP 地址；由 ObjectURL / PresignGet 在读时生成，不要入库
//
// # 语义约定
//
//   - GetObject / StatObject：对象不存在返回 ErrNotFound
//   - DeleteObject：对象不存在时幂等返回 nil
//   - ObjectURL：固定 URL = PublicBaseURL + "/" + uri；未配置 PublicBaseURL 时报错；不校验对象是否存在
//   - PresignGet：仅支持只读预签名；不校验对象是否存在；expiry 必须 > 0 且不超过 MaxPresignExpiry（默认 7 天）
//   - endpoint：SDK 通信地址（可内网）；PublicBaseURL：对外固定访问基址（全局一份，多桶共用）
//   - key / uri 前导 "/" 会被去掉
//
// # 实现隔离
//
// MinIO（minio.go）、AWS S3（s3.go）与阿里云 OSS（alioss.go）完全独立实现，禁止共享业务逻辑。
// URI 解析与固定 URL 拼装（uri.go）为包级公共能力，与具体 SDK 无关。
//
// @author zwei
package filestorage
