// Package logger 提供统一日志初始化能力
//
// @author zwei
package logger

import (
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	colorCyan        = "\033[36m"
	colorYellow      = "\033[33m"
	colorGray        = "\033[90m"
	colorBrightGreen = "\033[92m"
	colorReset       = "\033[0m"
)

// callerSkip 日志调用者跳过层级，补偿 component/common/logger、log.go 中 write/get 等包装层
const callerSkip = 2

// logHub 持有分流后的 zap logger 与资源清理函数
type logHub struct {
	access    *zap.Logger
	info      *zap.Logger
	errorL    *zap.Logger
	debug     *zap.Logger
	sensitive *Sensitive
	closers   []io.Closer
	cfg       Config
}

var defaultHub atomic.Pointer[logHub]

// Sync 将缓冲日志刷入各输出目标，应在进程退出前调用（通常配合 defer）
func Sync() error {
	hub := defaultHub.Load()
	if hub == nil {
		return nil
	}
	var errs []error
	for _, l := range []*zap.Logger{hub.access, hub.info, hub.errorL, hub.debug} {
		if l == nil {
			continue
		}
		if err := ignoreSyncError(l.Sync()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// InitLogger 读取配置并初始化控制台 + 四文件日志路由，返回资源清理函数
func InitLogger(serviceName, env string, logCfg Config) (func() error, error) {
	if err := logCfg.Normalize(); err != nil {
		return nil, errors.Wrap(err, "日志配置校验失败")
	}

	hub, err := newLogHub(serviceName, env, logCfg)
	if err != nil {
		return nil, err
	}
	defaultHub.Store(hub)

	cleanup := func() error {
		syncErr := Sync()
		var closeErrs []error
		for _, c := range hub.closers {
			if c == nil {
				continue
			}
			if err := c.Close(); err != nil {
				closeErrs = append(closeErrs, err)
			}
		}
		return errors.Join(append([]error{syncErr}, closeErrs...)...)
	}
	return cleanup, nil
}

// newLogHub 构建控制台 + 四文件分流的日志枢纽
func newLogHub(serviceName, env string, cfg Config) (*logHub, error) {
	consoleLevel := parseLevel(cfg.ConsoleLevel, zapcore.DebugLevel)
	fileLevel := parseLevel(cfg.Level, zapcore.InfoLevel)
	consoleCore := newConsoleCore(&colorConsoleWriter{w: os.Stdout}, consoleLevel)

	commonFields := []zap.Field{
		zap.String("service", serviceName),
		zap.String("env", env),
	}
	for k, v := range cfg.Fields {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		commonFields = append(commonFields, zap.String(key, val))
	}

	makeRot := func(logType string) (*FileRotator, error) {
		return NewFileRotator(RotatorOptions{
			RootDir:       cfg.Dir,
			LogType:       logType,
			MaxSizeMB:     cfg.Rotation.MaxSizeMB,
			RetentionDays: cfg.Rotation.RetentionDays,
			MaxBackups:    cfg.Rotation.MaxBackups,
			MaxTotalMB:    cfg.Rotation.MaxTotalMB,
			Compress:      cfg.Rotation.CompressOrDefault(),
		})
	}

	hub := &logHub{cfg: cfg}
	sens, err := NewSensitive(cfg.SensitiveMatch, cfg.SensitiveKeys)
	if err != nil {
		return nil, errors.Wrap(err, "初始化 sensitive 失败")
	}
	hub.sensitive = sens

	accessRot, err := makeRot(LogTypeAccess)
	if err != nil {
		return nil, errors.Wrap(err, "初始化 access 日志失败")
	}
	infoRot, err := makeRot(LogTypeInfo)
	if err != nil {
		_ = accessRot.Close()
		return nil, errors.Wrap(err, "初始化 info 日志失败")
	}
	errorRot, err := makeRot(LogTypeError)
	if err != nil {
		_ = accessRot.Close()
		_ = infoRot.Close()
		return nil, errors.Wrap(err, "初始化 error 日志失败")
	}
	hub.closers = append(hub.closers, accessRot, infoRot, errorRot)

	accessCores := []zapcore.Core{
		newJSONFileCore(accessRot, exactLevel(zapcore.InfoLevel), LogTypeAccess),
	}
	if cfg.Access.ToConsole {
		accessCores = append(accessCores, consoleCore)
	}
	hub.access = zap.New(zapcore.NewTee(accessCores...), zap.AddCaller(), zap.AddCallerSkip(callerSkip)).With(commonFields...)

	infoFileEnabler := andLevel(infoOrWarnLevel(), levelEnablerFrom(fileLevel))
	hub.info = zap.New(
		zapcore.NewTee(consoleCore, newJSONFileCore(infoRot, infoFileEnabler, LogTypeInfo)),
		zap.AddCaller(),
		zap.AddCallerSkip(callerSkip),
	).With(commonFields...)

	hub.errorL = zap.New(
		zapcore.NewTee(consoleCore, newJSONFileCore(errorRot, errorAndAbove(), LogTypeError)),
		zap.AddCaller(),
		zap.AddCallerSkip(callerSkip),
	).With(commonFields...)

	if fileLevel <= zapcore.DebugLevel {
		debugRot, err := makeRot(LogTypeDebug)
		if err != nil {
			return nil, errors.Wrap(err, "初始化 debug 日志失败")
		}
		hub.closers = append(hub.closers, debugRot)
		hub.debug = zap.New(
			zapcore.NewTee(
				consoleCore,
				newJSONFileCore(debugRot, exactLevel(zapcore.DebugLevel), LogTypeDebug),
			),
			zap.AddCaller(),
			zap.AddCallerSkip(callerSkip),
		).With(commonFields...)
	} else {
		// 文件不写 debug；控制台仍可按 console_level 输出 debug
		hub.debug = zap.New(
			newConsoleCore(&colorConsoleWriter{w: os.Stdout}, andLevel(consoleLevel, exactLevel(zapcore.DebugLevel))),
			zap.AddCaller(),
			zap.AddCallerSkip(callerSkip),
		).With(commonFields...)
	}

	return hub, nil
}

// newConsoleCore 创建控制台彩色文本输出 core
func newConsoleCore(writer io.Writer, level zapcore.LevelEnabler) zapcore.Core {
	encCfg := zap.NewDevelopmentEncoderConfig()
	encCfg.EncodeTime = colorTimeEncoder
	encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encCfg.EncodeCaller = colorCallerEncoder
	encCfg.EncodeName = colorNameEncoder
	return zapcore.NewCore(
		newTracingConsoleEncoder(encCfg),
		zapcore.AddSync(writer),
		level,
	)
}

// newJSONFileCore 创建带 log_type 字段的 NDJSON 文件 core（ELK 字段合同）
func newJSONFileCore(writer io.Writer, level zapcore.LevelEnabler, logType string) zapcore.Core {
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "time"
	encCfg.MessageKey = "message"
	encCfg.LevelKey = "level"
	encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encCfg),
		zapcore.AddSync(writer),
		level,
	)
	return core.With([]zap.Field{zap.String("log_type", logType)})
}

// parseLevel 将级别字符串解析为 zapcore.Level，无法识别时返回 defaultLevel
func parseLevel(levelText string, defaultLevel zapcore.Level) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(levelText)) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return defaultLevel
	}
}

// exactLevel 仅放行指定级别
func exactLevel(level zapcore.Level) zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l == level })
}

// infoOrWarnLevel 放行 Info 与 Warn（写入 info 文件）
func infoOrWarnLevel() zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return l == zapcore.InfoLevel || l == zapcore.WarnLevel
	})
}

// errorAndAbove 放行 Error 及以上
func errorAndAbove() zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l >= zapcore.ErrorLevel })
}

// levelEnablerFrom 放行不低于 min 的级别
func levelEnablerFrom(min zapcore.Level) zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l >= min })
}

// andLevel 同时满足两个 LevelEnabler
func andLevel(a, b zapcore.LevelEnabler) zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(l zapcore.Level) bool { return a.Enabled(l) && b.Enabled(l) })
}

// colorTimeEncoder 输出带青色的时间
func colorTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(colorCyan + t.Format("2006-01-02T15:04:05.000-0700") + colorReset)
}

// colorCallerEncoder 输出带黄色的调用者路径
func colorCallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(colorYellow + caller.TrimmedPath() + colorReset)
}

// colorNameEncoder 输出带灰色的 logger 名称
func colorNameEncoder(name string, enc zapcore.PrimitiveArrayEncoder) {
	if name == "" {
		return
	}
	enc.AppendString(colorGray + name + colorReset)
}

// colorConsoleWriter 控制台 Writer 包装，便于统一错误包装
type colorConsoleWriter struct {
	w io.Writer
}

// Write 写入控制台
func (c *colorConsoleWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if err != nil {
		return n, errors.Wrap(err, "写入控制台日志失败")
	}
	return n, nil
}

// ignoreSyncError 忽略 stdout/stderr 上常见的 sync 无效参数错误
func ignoreSyncError(err error) error {
	if err == nil {
		return nil
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) &&
		(pathErr.Path == "stdout" || pathErr.Path == "stderr" || pathErr.Path == "/dev/stdout" || pathErr.Path == "/dev/stderr") {
		return nil
	}
	return errors.Wrap(err, "刷新日志缓冲失败")
}

// CurrentAccessConfig 返回当前 access 配置副本（未初始化时返回零值，EnabledOrDefault 仍为 true）
func CurrentAccessConfig() AccessConfig {
	hub := defaultHub.Load()
	if hub == nil {
		return AccessConfig{}
	}
	return hub.cfg.Access
}

// currentSensitive 返回当前文件日志 sensitive 处理器；未初始化时透传（空 keys）
func currentSensitive() *Sensitive {
	hub := defaultHub.Load()
	if hub == nil || hub.sensitive == nil {
		return emptySensitive()
	}
	return hub.sensitive
}
