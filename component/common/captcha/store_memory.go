// 内存验证码存储。
//
// @author zwei

package captcha

import (
	"context"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// memoryEntry 是内存验证码条目
type memoryEntry struct {
	answer   string
	expireAt time.Time
}

// memoryStore 是基于 map 的进程内存储
type memoryStore struct {
	mu         sync.Mutex
	entries    map[string]memoryEntry
	maxEntries int
	now        func() time.Time
}

var _ Store = (*memoryStore)(nil)

// NewMemoryStore 创建内存验证码存储；仅适用于单实例或粘性会话
func NewMemoryStore(cfg MemoryStoreConfig) (Store, error) {
	maxEntries := cfg.MaxEntries
	if maxEntries == 0 {
		maxEntries = defaultMaxEntries
	}
	if maxEntries < minMaxEntries || maxEntries > maxMaxEntries {
		return nil, errors.Newf("captcha: max_entries 必须在 %d..%d 之间", minMaxEntries, maxMaxEntries)
	}

	return &memoryStore{
		entries:    make(map[string]memoryEntry, maxEntries),
		maxEntries: maxEntries,
		now:        time.Now,
	}, nil
}

// Save 保存答案；达到上限时先清理过期项，仍满则返回 ErrStoreFull
func (s *memoryStore) Save(ctx context.Context, id, answer string, ttl time.Duration) error {
	if err := validateStoreRecord(id, answer, ttl); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[id]; !exists && len(s.entries) >= s.maxEntries {
		s.removeExpiredLocked(now)
		if len(s.entries) >= s.maxEntries {
			return errors.Wrap(ErrStoreFull, "captcha: memory store 已达上限 %d", s.maxEntries)
		}
	}

	s.entries[id] = memoryEntry{answer: answer, expireAt: now.Add(ttl)}
	return nil
}

// Consume 原子取出并删除；过期项按未找到处理
func (s *memoryStore) Consume(ctx context.Context, id string) (string, bool, error) {
	if err := validateConsumeID(id); err != nil {
		return "", false, err
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.entries[id]
	if !exists {
		return "", false, nil
	}
	delete(s.entries, id)
	if !entry.expireAt.After(now) {
		return "", false, nil
	}
	return entry.answer, true, nil
}

// removeExpiredLocked 删除已过期条目；调用方须持锁
func (s *memoryStore) removeExpiredLocked(now time.Time) {
	for id, entry := range s.entries {
		if !entry.expireAt.After(now) {
			delete(s.entries, id)
		}
	}
}
