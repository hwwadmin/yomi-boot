// 公共入参校验。
//
// @author zwei

package vectordb

import (
	"context"
	"regexp"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

var collectionNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,254}$`)

// NormalizeCollectionName 修剪并校验集合名，返回规范化名称
func NormalizeCollectionName(name string) (string, error) {
	return normalizeCollectionName(name)
}

// normalizeCollectionName 修剪并校验集合名
func normalizeCollectionName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" || !collectionNameRe.MatchString(n) {
		return "", errors.Sentinel(ErrInvalidArgument)
	}
	return n, nil
}

// validateRecords 校验写入记录；同一批 ID 不得重复
func validateRecords(records []Record, dim int) error {
	if len(records) == 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	seen := make(map[string]struct{}, len(records))
	for i := range records {
		id := strings.TrimSpace(records[i].ID)
		if id == "" || len(id) > maxIDLen {
			return errors.Sentinel(ErrInvalidArgument)
		}
		if _, ok := seen[id]; ok {
			return errors.Sentinel(ErrInvalidArgument)
		}
		seen[id] = struct{}{}
		if len(records[i].Vector) != dim {
			return errors.Sentinel(ErrInvalidArgument)
		}
		for k := range records[i].Metadata {
			if strings.TrimSpace(k) == "" {
				return errors.Sentinel(ErrInvalidArgument)
			}
		}
	}
	return nil
}

// validateIDs 校验 ID 列表
func validateIDs(ids []string) error {
	if len(ids) == 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for _, id := range ids {
		s := strings.TrimSpace(id)
		if s == "" || len(s) > maxIDLen {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}

// validateFilter 校验等值过滤；requireNonEmpty 为 true 时至少 1 个键
func validateFilter(filter map[string]string, requireNonEmpty bool) error {
	if requireNonEmpty && len(filter) == 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for k := range filter {
		key := strings.TrimSpace(k)
		if key == "" || strings.ContainsAny(key, `"\\`) {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}

// validateSearchQuery 校验检索入参
func validateSearchQuery(q SearchQuery, dim int) error {
	if len(q.Vector) != dim {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if q.TopK <= 0 || q.TopK > MaxTopK {
		return errors.Sentinel(ErrInvalidArgument)
	}
	return validateFilter(q.Filter, false)
}

// withTimeout 在父 ctx 上套本包超时
func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, timeoutOrDefault(d))
}

// copyMetadata 拷贝元数据，避免与调用方共享 map
func copyMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[strings.TrimSpace(k)] = v
	}
	return dst
}

// copyVector 拷贝向量
func copyVector(src []float32) []float32 {
	if src == nil {
		return nil
	}
	dst := make([]float32, len(src))
	copy(dst, src)
	return dst
}

// matchFilter 判断 metadata 是否满足等值 AND；空 filter 视为通过
func matchFilter(metadata, filter map[string]string) bool {
	if len(filter) == 0 {
		return true
	}
	for k, v := range filter {
		key := strings.TrimSpace(k)
		if metadata[key] != v {
			return false
		}
	}
	return true
}
