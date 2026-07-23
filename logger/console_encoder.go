// Package logger 提供统一日志初始化能力
//
// @author zwei
package logger

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/shared"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// tracingConsoleEncoder 带链路追踪 ID 的控制台日志编码器
type tracingConsoleEncoder struct {
	cfg          zapcore.EncoderConfig
	fieldEncoder zapcore.Encoder
}

// newTracingConsoleEncoder 创建带链路追踪 ID 的控制台编码器
func newTracingConsoleEncoder(cfg zapcore.EncoderConfig) zapcore.Encoder {
	return &tracingConsoleEncoder{
		cfg:          cfg,
		fieldEncoder: zapcore.NewJSONEncoder(fieldsOnlyJSONEncoderConfig(cfg)),
	}
}

// fieldsOnlyJSONEncoderConfig 去掉 Entry 元数据键，控制台 JSON 段只保留业务字段
func fieldsOnlyJSONEncoderConfig(cfg zapcore.EncoderConfig) zapcore.EncoderConfig {
	fieldCfg := cfg
	fieldCfg.TimeKey = ""
	fieldCfg.LevelKey = ""
	fieldCfg.NameKey = ""
	fieldCfg.CallerKey = ""
	fieldCfg.FunctionKey = ""
	fieldCfg.MessageKey = ""
	fieldCfg.StacktraceKey = ""
	return fieldCfg
}

// EncodeEntry 将日志条目编码为控制台格式的 buffer（含时间、级别、traceID、调用者）
func (e *tracingConsoleEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	traceID, otherFields := splitMetaFields(fields)

	line := buffer.NewPool().Get()
	appendBracketField(line, encodeTimeString(ent.Time))
	appendBracketField(line, encodeLevelString(e.cfg, ent.Level))
	appendBracketField(line, colorBrightGreen+traceID+colorReset)
	appendBracketField(line, encodeCallerString(ent.Caller))
	line.AppendString(ent.Message)

	if len(otherFields) > 0 {
		fieldBuf, err := e.fieldEncoder.EncodeEntry(zapcore.Entry{}, otherFields)
		if err != nil {
			return line, errors.Wrap(err, "编码控制台日志字段失败")
		}
		line.AppendByte(' ')
		appendBracketField(line, colorBrightGreen+strings.TrimSpace(fieldBuf.String())+colorReset)
		fieldBuf.Free()
	}

	line.AppendString(e.cfg.LineEnding)
	return line, nil
}

// appendBracketField 将值以 [value] 格式追加到 buffer
func appendBracketField(line *buffer.Buffer, value string) {
	line.AppendByte('[')
	line.AppendString(value)
	line.AppendByte(']')
	line.AppendByte(' ')
}

// encodeTimeString 将时间编码为彩色字符串
func encodeTimeString(t time.Time) string {
	enc := &stringArrayEncoder{}
	colorTimeEncoder(t, enc)
	return enc.String()
}

// encodeLevelString 将日志级别编码为彩色字符串
func encodeLevelString(cfg zapcore.EncoderConfig, level zapcore.Level) string {
	enc := &stringArrayEncoder{}
	cfg.EncodeLevel(level, enc)
	return enc.String()
}

// encodeCallerString 将调用者信息编码为彩色字符串
func encodeCallerString(caller zapcore.EntryCaller) string {
	enc := &stringArrayEncoder{}
	colorCallerEncoder(caller, enc)
	return strings.TrimSpace(enc.String())
}

// stringArrayEncoder 实现 zapcore.PrimitiveArrayEncoder，仅收集字符串值
// 用于将编码器输出临时捕获为字符串
// 非字符串类型的方法均为空实现

type stringArrayEncoder struct {
	buf string
}

// AppendBool 追加布尔值
func (s *stringArrayEncoder) AppendBool(bool) {}

// AppendByteString 追加字节串
func (s *stringArrayEncoder) AppendByteString(v []byte) { s.buf += string(v) }

// AppendComplex128 追加 complex128
func (s *stringArrayEncoder) AppendComplex128(complex128) {}

// AppendComplex64 追加 complex64
func (s *stringArrayEncoder) AppendComplex64(complex64) {}

// AppendFloat64 追加 float64
func (s *stringArrayEncoder) AppendFloat64(float64) {}

// AppendFloat32 追加 float32
func (s *stringArrayEncoder) AppendFloat32(float32) {}

// AppendInt 追加 int
func (s *stringArrayEncoder) AppendInt(int) {}

// AppendInt64 追加 int64
func (s *stringArrayEncoder) AppendInt64(int64) {}

// AppendInt32 追加 int32
func (s *stringArrayEncoder) AppendInt32(int32) {}

// AppendInt16 追加 int16
func (s *stringArrayEncoder) AppendInt16(int16) {}

// AppendInt8 追加 int8
func (s *stringArrayEncoder) AppendInt8(int8) {}

// AppendString 追加字符串
func (s *stringArrayEncoder) AppendString(v string) { s.buf += v }

// AppendUint 追加 uint
func (s *stringArrayEncoder) AppendUint(uint) {}

// AppendUint64 追加 uint64
func (s *stringArrayEncoder) AppendUint64(uint64) {}

// AppendUint32 追加 uint32
func (s *stringArrayEncoder) AppendUint32(uint32) {}

// AppendUint16 追加 uint16
func (s *stringArrayEncoder) AppendUint16(uint16) {}

// AppendUint8 追加 uint8
func (s *stringArrayEncoder) AppendUint8(uint8) {}

// AppendUintptr 追加 uintptr
func (s *stringArrayEncoder) AppendUintptr(uintptr) {}

// String 返回累积的字符串
func (s *stringArrayEncoder) String() string {
	return s.buf
}

// Clone 返回编码器的深拷贝
func (e *tracingConsoleEncoder) Clone() zapcore.Encoder {
	return &tracingConsoleEncoder{
		cfg:          e.cfg,
		fieldEncoder: e.fieldEncoder.Clone(),
	}
}

// AddArray 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddArray(key string, marshaler zapcore.ArrayMarshaler) error {
	return e.fieldEncoder.AddArray(key, marshaler)
}

// AddObject 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddObject(key string, marshaler zapcore.ObjectMarshaler) error {
	return e.fieldEncoder.AddObject(key, marshaler)
}

// AddBinary 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddBinary(key string, value []byte) {
	e.fieldEncoder.AddBinary(key, value)
}

// AddByteString 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddByteString(key string, value []byte) {
	e.fieldEncoder.AddByteString(key, value)
}

// AddBool 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddBool(key string, value bool) {
	e.fieldEncoder.AddBool(key, value)
}

// AddComplex128 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddComplex128(key string, value complex128) {
	e.fieldEncoder.AddComplex128(key, value)
}

// AddComplex64 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddComplex64(key string, value complex64) {
	e.fieldEncoder.AddComplex64(key, value)
}

// AddDuration 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddDuration(key string, value time.Duration) {
	e.fieldEncoder.AddDuration(key, value)
}

// AddFloat64 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddFloat64(key string, value float64) {
	e.fieldEncoder.AddFloat64(key, value)
}

// AddFloat32 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddFloat32(key string, value float32) {
	e.fieldEncoder.AddFloat32(key, value)
}

// AddInt 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddInt(key string, value int) {
	e.fieldEncoder.AddInt(key, value)
}

// AddInt64 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddInt64(key string, value int64) {
	e.fieldEncoder.AddInt64(key, value)
}

// AddInt32 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddInt32(key string, value int32) {
	e.fieldEncoder.AddInt32(key, value)
}

// AddInt16 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddInt16(key string, value int16) {
	e.fieldEncoder.AddInt16(key, value)
}

// AddInt8 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddInt8(key string, value int8) {
	e.fieldEncoder.AddInt8(key, value)
}

// AddString 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddString(key, value string) {
	e.fieldEncoder.AddString(key, value)
}

// AddTime 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddTime(key string, value time.Time) {
	e.fieldEncoder.AddTime(key, value)
}

// AddUint 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddUint(key string, value uint) {
	e.fieldEncoder.AddUint(key, value)
}

// AddUint64 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddUint64(key string, value uint64) {
	e.fieldEncoder.AddUint64(key, value)
}

// AddUint32 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddUint32(key string, value uint32) {
	e.fieldEncoder.AddUint32(key, value)
}

// AddUint16 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddUint16(key string, value uint16) {
	e.fieldEncoder.AddUint16(key, value)
}

// AddUint8 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddUint8(key string, value uint8) {
	e.fieldEncoder.AddUint8(key, value)
}

// AddUintptr 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddUintptr(key string, value uintptr) {
	e.fieldEncoder.AddUintptr(key, value)
}

// AddReflected 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) AddReflected(key string, value interface{}) error {
	return e.fieldEncoder.AddReflected(key, value)
}

// OpenNamespace 委托给内嵌的 JSON 字段编码器
func (e *tracingConsoleEncoder) OpenNamespace(key string) {
	e.fieldEncoder.OpenNamespace(key)
}

// splitMetaFields 从字段列表中提取 traceID，其余字段作为业务字段返回
// span_id / parent_span_id 视为系统元数据，不进入控制台 JSON 段（控制台仍以 trace 展示为主）
func splitMetaFields(fields []zapcore.Field) (traceID string, rest []zapcore.Field) {
	traceID = shared.MissingTraceID
	rest = make([]zapcore.Field, 0, len(fields))

	for _, field := range fields {
		switch field.Key {
		case shared.TraceID:
			if field.Type == zapcore.StringType && strings.TrimSpace(field.String) != "" {
				traceID = field.String
			}
			continue
		case shared.SpanID, shared.ParentSpanID:
			continue
		}
		rest = append(rest, field)
	}

	if strings.TrimSpace(traceID) == "" {
		traceID = shared.MissingTraceID
	}
	return traceID, rest
}
