// Package httputil 标准库 HTTP 请求相关的工具函数
//
// @author zwei
package httputil

import (
	"net"
	"net/http"
	"strings"
)

// RemoteAddrHost 从 HTTP 请求的 RemoteAddr 解析客户端主机地址（不解析代理头）
func RemoteAddrHost(r *http.Request) string {
	if r == nil {
		return ""
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}
