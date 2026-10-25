package logger

import (
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/masking"

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

// callerSkip 日志调用者跳过层级，补偿 component/core/logger、log.go 中 write/get 等包装层
const callerSkip = 2

// logHub 持有分流后的 zap logger 与异步 sink / 文件滚动器
type logHub struct {
	access   *zap.Logger
	info     *zap.Logger
	errorL   *zap.Logger
	debug    *zap.Logger
	maskSvc  *masking.Service
	rotators []*FileRotator // rotators 是全部文件滚动器，停机时 flush 后 sync+close
	sinks    []*asyncSink   // sinks 是全部异步 sink，停机时统一 flush/stop
	cfg      Config
}

var defaultHub atomic.Pointer[logHub]

// Flush 将各异步 sink 中在途日志落盘；timeout 为总预算，
// 单个 sink 超时返回错误但不影响其余 sink。异步关闭时为无操作。
func Flush(timeout time.Duration) error {
	hub := defaultHub.Load()
	if hub == nil {
		return nil
	}
	return hub.Flush(timeout)
}

// Flush 按总预算逐个刷新 sink；超时的 sink 记入错误
func (h *logHub) Flush(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultFlushTimeout
	}
	deadline := time.Now().Add(timeout)
	var errs []error
	for _, s := range h.sinks {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			errs = append(errs, errors.Newf("logger: sink[%s] flush 预算耗尽", s.name))
			continue
		}
		if err := s.Flush(remaining); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Sync 将缓冲日志刷入各输出目标，应在进程退出前调用（通常配合 defer）
func Sync() error {
	hub := defaultHub.Load()
	if hub == nil {
		return nil
	}
	flushErr := hub.Flush(defaultFlushTimeout)
	var errs []error
	for _, r := range hub.rotators {
		if err := ignoreSyncError(r.Sync()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(append([]error{flushErr}, errs...)...)
}

// InitLogger 读取配置并初始化控制台 + 四文件日志路由，返回资源清理函数。
// maskSvc 是注入的统一脱敏服务；是否调用由 cfg.MaskSensitive 在组件内聚判定。
func InitLogger(serviceName, env string, logCfg Config, maskSvc *masking.Service) (func() error, error) {
	if err := logCfg.Normalize(); err != nil {
		return nil, errors.Wrap(err, "日志配置校验失败")
	}
	// 脱敏开关内聚在组件：关闭时不持有脱敏服务，日志全部原文直出
	if !logCfg.MaskSensitive {
		maskSvc = nil
	}

	hub, err := newLogHub(serviceName, env, logCfg, maskSvc)
	if err != nil {
		return nil, err
	}
	defaultHub.Store(hub)

	cleanup := func() error {
		syncErr := Sync()
		var closeErrs []error
		for _, s := range hub.sinks {
			s.Stop()
		}
		for _, r := range hub.rotators {
			if err := r.Close(); err != nil {
				closeErrs = append(closeErrs, err)
			}
		}
		return errors.Join(append([]error{syncErr}, closeErrs...)...)
	}
	return cleanup, nil
}

// newLogHub 构建控制台 + 四文件分流的日志枢纽
func newLogHub(serviceName, env string, cfg Config, maskSvc *masking.Service) (*logHub, error) {
	fileLevel := parseLevel(cfg.Level, zapcore.InfoLevel)

	hub := &logHub{cfg: cfg, maskSvc: maskSvc}
	queueSize := cfg.AsyncQueueSize

	// 控制台关闭时不构建 consoleCore，所有 Tee 仅挂文件 core；
	// 控制台等级跟随 level，debug 日志仅 level=debug 时上控制台
	var consoleCore zapcore.Core
	if cfg.ConsoleEnabledOrDefault() {
		var consoleWriter io.Writer = &colorConsoleWriter{w: os.Stdout}
		if cfg.AsyncEnabledOrDefault() {
			consoleWriter = hub.addSink("console", consoleWriter, queueSize)
		}
		consoleCore = newConsoleCore(consoleWriter, levelEnablerFrom(fileLevel))
	}

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

	accessRot, err := makeRot(LogTypeAccess)
	if err != nil {
		return nil, errors.Wrap(err, "初始化 access 日志失败")
	}
	hub.rotators = append(hub.rotators, accessRot)
	infoRot, err := makeRot(LogTypeInfo)
	if err != nil {
		hub.abort()
		return nil, errors.Wrap(err, "初始化 info 日志失败")
	}
	hub.rotators = append(hub.rotators, infoRot)
	errorRot, err := makeRot(LogTypeError)
	if err != nil {
		hub.abort()
		return nil, errors.Wrap(err, "初始化 error 日志失败")
	}
	hub.rotators = append(hub.rotators, errorRot)

	// wrapSink 按异步开关包装文件目标；异步关闭时直连（同步直写语义）
	wrapSink := func(logType string, rot *FileRotator) io.Writer {
		if !cfg.AsyncEnabledOrDefault() {
			return rot
		}
		return hub.addSink(logType, rot, queueSize)
	}

	accessCores := []zapcore.Core{
		newJSONFileCore(wrapSink(LogTypeAccess, accessRot), exactLevel(zapcore.InfoLevel), LogTypeAccess),
	}
	if cfg.Access.ToConsole && consoleCore != nil {
		accessCores = append(accessCores, consoleCore)
	}
	hub.access = zap.New(zapcore.NewTee(accessCores...), zap.AddCaller(), zap.AddCallerSkip(callerSkip)).With(commonFields...)

	infoFileEnabler := andLevel(infoOrWarnLevel(), levelEnablerFrom(fileLevel))
	infoCores := []zapcore.Core{newJSONFileCore(wrapSink(LogTypeInfo, infoRot), infoFileEnabler, LogTypeInfo)}
	if consoleCore != nil {
		infoCores = append(infoCores, consoleCore)
	}
	hub.info = zap.New(zapcore.NewTee(infoCores...), zap.AddCaller(), zap.AddCallerSkip(callerSkip)).With(commonFields...)

	errorCores := []zapcore.Core{newJSONFileCore(wrapSink(LogTypeError, errorRot), errorAndAbove(), LogTypeError)}
	if consoleCore != nil {
		errorCores = append(errorCores, consoleCore)
	}
	hub.errorL = zap.New(zapcore.NewTee(errorCores...), zap.AddCaller(), zap.AddCallerSkip(callerSkip)).With(commonFields...)

	if fileLevel <= zapcore.DebugLevel {
		debugRot, err := makeRot(LogTypeDebug)
		if err != nil {
			hub.abort()
			return nil, errors.Wrap(err, "初始化 debug 日志失败")
		}
		hub.rotators = append(hub.rotators, debugRot)
		debugCores := []zapcore.Core{newJSONFileCore(wrapSink(LogTypeDebug, debugRot), exactLevel(zapcore.DebugLevel), LogTypeDebug)}
		if consoleCore != nil {
			debugCores = append(debugCores, consoleCore)
		}
		hub.debug = zap.New(zapcore.NewTee(debugCores...), zap.AddCaller(), zap.AddCallerSkip(callerSkip)).With(commonFields...)
	} else {
		// level 高于 debug 时 debug 日志无处输出；保留空 logger 维持调用方 nil 安全
		hub.debug = zap.New(zapcore.NewTee(), zap.AddCaller(), zap.AddCallerSkip(callerSkip)).With(commonFields...)
	}

	return hub, nil
}

// addSink 为目标创建异步 sink 并登记，供 flush/stop 统一管理
func (h *logHub) addSink(name string, dst io.Writer, queueSize int) io.Writer {
	sink := newAsyncSink(name, dst, queueSize)
	h.sinks = append(h.sinks, sink)
	return sink
}

// abort 构建失败路径：停止已创建的 sink 并关闭已创建的滚动器
func (h *logHub) abort() {
	for _, s := range h.sinks {
		s.Stop()
	}
	for _, r := range h.rotators {
		_ = r.Close()
	}
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

// currentMasking 返回当前文件日志的脱敏服务；未注入时为 nil（方法 nil 安全，等价透传）
func currentMasking() *masking.Service {
	hub := defaultHub.Load()
	if hub == nil {
		return nil
	}
	return hub.maskSvc
}
