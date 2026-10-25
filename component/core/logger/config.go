package logger

import (
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// LogType 文件日志类型
const (
	LogTypeAccess = "access"
	LogTypeInfo   = "info"
	LogTypeError  = "error"
	LogTypeDebug  = "debug"
)

// reservedFieldKeys 禁止出现在 log.fields 中的保留键
var reservedFieldKeys = map[string]struct{}{
	"time": {}, "level": {}, "log_type": {}, "service": {}, "env": {},
	"message": {}, "msg": {}, "trace_id": {}, "traceID": {},
	"span_id": {}, "parent_span_id": {},
	"caller": {}, "stack": {}, "name": {},
}

// RotationConfig 文件滚动与保留配置（须显式配置，无策略档）
type RotationConfig struct {
	MaxSizeMB     int   // 单文件大小上限（MB）
	RetentionDays int   // 历史保留天数
	MaxBackups    int   // 历史文件个数上限（按日志类型目录分别计，每类保留最新 N 个）
	MaxTotalMB    int   // 整个 logs 根目录总容量上限（MB），统计含活跃日志文件；超出时从最旧历史开始删除
	Compress      *bool // 历史是否 gzip；nil 默认 true
}

// CompressOrDefault 历史文件是否 gzip，默认 true
func (r RotationConfig) CompressOrDefault() bool {
	if r.Compress == nil {
		return true
	}
	return *r.Compress
}

// AccessConfig 文件 access 中间件配置（与 httprecord Kafka 审计无关）
type AccessConfig struct {
	Enabled       *bool    // nil 默认开启
	SkipPaths     []string // 不记录的路径前缀/精确路径
	MaxBodyBytes  int      // body 采集上限
	MaxFieldBytes int      // 单字段（如 header 值）上限
	RecordBody    bool     // 是否采集 body
	ToConsole     bool     // access 是否同时打控制台
	SampleRate    float64  // 默认 1 全量；(0,1) 仅约束成功请求；>=400 必采
}

// EnabledOrDefault access 文件日志默认开启
func (c AccessConfig) EnabledOrDefault() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// SampleRateOrDefault 返回 access 采样率；未配/<=0/>1 均视为 1（全量）
func (c AccessConfig) SampleRateOrDefault() float64 {
	if c.SampleRate <= 0 || c.SampleRate > 1 {
		return 1
	}
	return c.SampleRate
}

// Config 表示日志输出配置
//
// 日志时间统一使用进程时区（启动时由 app.timezone 锁定，默认 UTC）；原 log.timezone 已移除。
type Config struct {
	Dir           string            // 日志根目录（其下 access/info/error/debug）
	Level         string            // 文件最低级别
	ConsoleLevel  string            // 控制台最低级别
	Fields        map[string]string // 静态标签，自动注入每条日志
	MaskSensitive bool              // MaskSensitive 是否用注入的脱敏服务处理日志；false 时不调用脱敏组件（组件内聚判定）
	Rotation      RotationConfig    // 滚动与保留（必填）
	Access        AccessConfig      // 文件 access 中间件
}

// Normalize 填充安全默认值并校验硬性约束
func (c *Config) Normalize() error {
	if c == nil {
		return errors.New("config 不能为 nil")
	}
	if strings.TrimSpace(c.Dir) == "" {
		c.Dir = "logs"
	}
	if strings.TrimSpace(c.Level) == "" {
		c.Level = "info"
	}
	if strings.TrimSpace(c.ConsoleLevel) == "" {
		c.ConsoleLevel = "debug"
	}

	if c.Rotation.MaxSizeMB <= 0 || c.Rotation.RetentionDays <= 0 || c.Rotation.MaxBackups <= 0 {
		return errors.New("rotation.max_size_mb/retention_days/max_backups 必须显式配置且 > 0")
	}
	if c.Rotation.MaxTotalMB <= 0 {
		return errors.New("rotation.max_total_mb 必须显式配置且 > 0")
	}

	if err := validateFields(c.Fields); err != nil {
		return err
	}

	if c.Access.MaxBodyBytes <= 0 {
		c.Access.MaxBodyBytes = 8192
	}
	if c.Access.MaxFieldBytes <= 0 {
		c.Access.MaxFieldBytes = 1024
	}
	if len(c.Access.SkipPaths) == 0 {
		c.Access.SkipPaths = []string{"/health", "/metrics", "/ready", "/live"}
	}
	// 默认全量；0 或越界归一为 1，避免「0=不采」歧义
	c.Access.SampleRate = c.Access.SampleRateOrDefault()
	return nil
}

// validateFields 校验 fields 不得覆盖系统保留键
func validateFields(fields map[string]string) error {
	for k := range fields {
		key := strings.TrimSpace(k)
		if key == "" {
			return errors.New("fields 键不能为空")
		}
		if _, banned := reservedFieldKeys[key]; banned {
			return errors.Newf("fields 禁止覆盖保留键: %s", key)
		}
	}
	return nil
}
