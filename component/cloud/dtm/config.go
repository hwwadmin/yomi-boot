// Package dtm 提供 DTM 服务器连接配置
//
// @author zwei
package dtm

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const dtmAPIPath = "/api/dtmsvr"

var barrierTablePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Config 保存 DTM 客户端与 Barrier 表配置
type Config struct {
	Server       string // Server 是 DTM 服务器地址
	Timeout      int    // Timeout 是控制面请求超时秒数，未配置默认 3
	BarrierTable string // BarrierTable 是 DTM Barrier 表名，未配置默认 sys_barrier
}

// TimeoutOrDefault 返回控制面请求超时，未配置或非正数时使用 DefaultTimeout
func (c Config) TimeoutOrDefault() time.Duration {
	if c.Timeout <= 0 {
		return DefaultTimeout
	}
	return time.Duration(c.Timeout) * time.Second
}

// BarrierTableOrDefault 返回 Barrier 表名，未配置时使用 DefaultBarrierTableName
func (c Config) BarrierTableOrDefault() string {
	table := strings.TrimSpace(c.BarrierTable)
	if table == "" {
		return DefaultBarrierTableName
	}
	return table
}

// Validate 校验 DTM 配置
func (c Config) Validate() error {
	if c.Timeout < 0 {
		return errors.New("timeout 不能为负数")
	}
	if table := strings.TrimSpace(c.BarrierTable); table != "" && !barrierTablePattern.MatchString(table) {
		return errors.Newf("barrier_table 非法: %s", table)
	}
	if strings.TrimSpace(c.Server) == "" {
		return nil
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
