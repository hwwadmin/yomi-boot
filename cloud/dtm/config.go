// Package dtm 提供 DTM 服务器连接配置
//
// @author zwei
package dtm

import (
	"net/url"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/config"
	"gitee.com/hq_project/yomi-boot/boot/errors"
)

const (
	// SectionKey YAML 根 key
	SectionKey = "dtm"

	dtmAPIPath = "/api/dtmsvr"
)

// Config 保存 DTM 服务器连接配置
type Config struct {
	Enabled *bool  `mapstructure:"enabled"` // Enabled 是否启用 DTM，未配置默认 false
	Server  string `mapstructure:"server"`  // Server 是 DTM 服务器地址
	Timeout int    `mapstructure:"timeout"` // Timeout 是控制面请求超时秒数，未配置默认 3
}

// EnabledOrDefault 返回是否启用 DTM，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// TimeoutOrDefault 返回控制面请求超时，未配置或非正数时使用 DefaultTimeout
func (c Config) TimeoutOrDefault() time.Duration {
	if c.Timeout <= 0 {
		return DefaultTimeout
	}
	return time.Duration(c.Timeout) * time.Second
}

// Validate 校验 DTM 配置（仅 enabled=true 时校验）
func (c Config) Validate() error {
	if c.Timeout < 0 {
		return errors.New("dtm.timeout 不能为负数")
	}
	if !c.EnabledOrDefault() {
		return nil
	}
	if strings.TrimSpace(c.Server) == "" {
		return errors.New("dtm.server 不能为空")
	}
	return validateHTTPURL("server", c.ResolveServer())
}

// ResolveServer 返回规范化后的 DTM 服务端地址（补全 http 协议与 /api/dtmsvr 路径）
func (c Config) ResolveServer() string {
	server := strings.TrimSpace(c.Server)
	if server == "" {
		return ""
	}
	if !strings.Contains(server, "://") {
		server = "http://" + server
	}
	u, err := url.Parse(server)
	if err != nil {
		return strings.TrimRight(server, "/")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = dtmAPIPath
	}
	return strings.TrimRight(u.String(), "/")
}

// ConfigSection 返回 dtm 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
