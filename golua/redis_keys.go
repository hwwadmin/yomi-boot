// Package golua Redis key 约定
//
// @author zwei
package golua

import "fmt"

// redisKeys 封装 golua 脚本缓存 key
type redisKeys struct {
	prefix string
}

// newRedisKeys 按前缀构造 key 助手
func newRedisKeys(prefix string) redisKeys {
	return redisKeys{prefix: prefix}
}

// gen 返回 current_gen key
func (k redisKeys) gen() string {
	return fmt.Sprintf("%s:current_gen", k.prefix)
}

// index 返回脚本 id 集合 key
func (k redisKeys) index() string {
	return fmt.Sprintf("%s:index", k.prefix)
}

// script 返回单脚本内容 key
func (k redisKeys) script(id string) string {
	return fmt.Sprintf("%s:script:%s", k.prefix, id)
}
