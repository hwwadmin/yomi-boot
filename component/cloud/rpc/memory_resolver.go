// @author zwei

package rpc

import (
	"context"
	"net/url"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// MemoryEndpointResolver 基于本地内存配置解析服务地址
// 配置为「服务名 -> 基 URL」，基 URL 须带 http:// 或 https://
type MemoryEndpointResolver struct {
	endpoints map[string]string
}

// NewMemoryEndpointResolver 创建本地内存端点解析器
// 构造时拷贝并规整配置；解析器与调用方后续对原 map 的修改隔离
func NewMemoryEndpointResolver(endpoints map[string]string) *MemoryEndpointResolver {
	copied := make(map[string]string, len(endpoints))
	for serviceName, baseURL := range endpoints {
		serviceName = strings.TrimSpace(serviceName)
		if serviceName == "" {
			continue
		}
		copied[serviceName] = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
	return &MemoryEndpointResolver{endpoints: copied}
}

// ResolveHTTPBase 按服务名查找基 URL 并拼接路径
func (r *MemoryEndpointResolver) ResolveHTTPBase(ctx context.Context, serviceName, pathSuffix string) (string, error) {
	if r == nil {
		return "", errors.New("MemoryEndpointResolver 未初始化")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name := strings.TrimSpace(serviceName)
	if name == "" {
		return "", errors.New("服务名不能为空")
	}
	baseURL, ok := r.endpoints[name]
	if !ok || baseURL == "" {
		return "", errors.Newf("未配置服务地址: %s", name)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.Newf("服务地址非法: %s", name)
	}
	suffix := strings.TrimRight(strings.TrimSpace(pathSuffix), "/")
	if suffix != "" && !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return baseURL + suffix, nil
}
