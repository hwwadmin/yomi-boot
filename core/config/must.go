// Package config 提供 Fail Fast 配置加载与校验
//
// @author zwei
package config

import (
	"fmt"
	"os"
)

// MustLoadDocument 按注册表加载并校验配置，失败时输出 stderr 并 panic
func MustLoadDocument(configPath, envPrefix string, reg *Registry) *Document {
	doc, err := LoadDocument(configPath, envPrefix, reg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "配置加载失败: %v\n", err)
		panic(err)
	}
	return doc
}
