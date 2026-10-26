// 按过滤键替换的共享辅助：旧主键计算与串行锁。
//
// @author zwei

package vectordb

import (
	"sort"
	"strings"
	"sync"
)

const (
	// replaceLockStripes 是替换锁条数；同一过滤键命中同一条带
	replaceLockStripes = 64
)

// replaceLock 按过滤键串行化替换操作；只在进程内生效
type replaceLock struct {
	mu [replaceLockStripes]sync.Mutex
}

// lock 获取过滤键对应的条带锁，返回释放函数
func (l *replaceLock) lock(collection string, filter map[string]string) func() {
	stripe := int(fnv32(collection+"\x00"+filterKey(filter)) % replaceLockStripes)
	l.mu[stripe].Lock()
	return l.mu[stripe].Unlock
}

// filterKey 将过滤条件序列化为稳定字符串
func filterKey(filter map[string]string) string {
	keys := make([]string, 0, len(filter))
	for k := range filter {
		keys = append(keys, strings.TrimSpace(k))
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(filter[k])
		b.WriteByte('\x00')
	}
	return b.String()
}

// fnv32 计算字符串的 FNV-1a 哈希
func fnv32(s string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	return h
}

// staleIDs 返回 filter 命中主键中不在 records 里的部分
func staleIDs(ids []string, records []Record) []string {
	keep := make(map[string]struct{}, len(records))
	for i := range records {
		keep[strings.TrimSpace(records[i].ID)] = struct{}{}
	}
	stale := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := keep[id]; ok {
			continue
		}
		stale = append(stale, id)
	}
	return stale
}
