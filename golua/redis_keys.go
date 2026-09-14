// Package golua Redis 键约定
//
// @author zwei
package golua

import "fmt"

// redisKeys Redis key 拼装
type redisKeys struct {
	prefix string
}

// newRedisKeys 按前缀构造
func newRedisKeys(prefix string) redisKeys {
	return redisKeys{prefix: prefix}
}

// gen 共享代际键
func (k redisKeys) gen() string {
	return fmt.Sprintf("%s:current_gen", k.prefix)
}

// index 脚本 id 集合键
func (k redisKeys) index() string {
	return fmt.Sprintf("%s:index", k.prefix)
}

// script 单脚本内容键
func (k redisKeys) script(id string) string {
	return fmt.Sprintf("%s:script:%s", k.prefix, id)
}

// lock 写路径分布式锁键
func (k redisKeys) lock() string {
	return fmt.Sprintf("%s:lock", k.prefix)
}
