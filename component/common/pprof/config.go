// Package pprof 提供 pprof 性能剖析配置
//
// @author zwei
package pprof

import (
	"net"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Config 表示 pprof 独立 debug 端口配置
type Config struct {
	Addr string // Addr 是监听地址，如 :16080
}

// ListenAddr 返回规范化后的监听地址
func (c Config) ListenAddr() string {
	return strings.TrimSpace(c.Addr)
}

// Validate 校验 pprof 配置语义
func (c Config) Validate() error {
	addr := c.ListenAddr()
	if addr == "" {
		return errors.New("addr 不能为空")
	}
	if _, err := net.ResolveTCPAddr("tcp", addr); err != nil {
		return errors.Wrap(err, "addr 不合法")
	}
	return nil
}
