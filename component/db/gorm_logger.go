// GORM 日志适配：把 SQL 日志接入项目 logger
//
// @author zwei

package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/logger"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newGormLogger 构建接入项目 logger 的 GORM 日志适配器（慢 SQL 阈值可配）
func newGormLogger(cfg Config) gormlogger.Interface {
	return &gormLog{
		level:         gormlogger.Warn,
		slowThreshold: cfg.SlowThreshold(),
	}
}

// gormLog 把 GORM 的 SQL 日志接入项目 logger：
// 慢 SQL 记 Warn、执行失败记 Error、记录不存在视为正常流不打印，且不输出绑定参数
type gormLog struct {
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

var (
	_ gormlogger.Interface = (*gormLog)(nil)
	_ gorm.ParamsFilter    = (*gormLog)(nil)
)

// LogMode 返回指定级别的日志器副本，不修改原实例
func (l *gormLog) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return &gormLog{level: level, slowThreshold: l.slowThreshold}
}

// Info 输出 GORM 普通日志；msg 为格式化模板
func (l *gormLog) Info(ctx context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Info {
		logger.Info(ctx, "SQL "+strings.TrimSpace(fmt.Sprintf(msg, data...)))
	}
}

// Warn 输出 GORM 告警日志；msg 为格式化模板
func (l *gormLog) Warn(ctx context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Warn {
		logger.Warn(ctx, "SQL "+strings.TrimSpace(fmt.Sprintf(msg, data...)))
	}
}

// Error 输出 GORM 错误日志；msg 为格式化模板
func (l *gormLog) Error(ctx context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Error {
		logger.Error(ctx, "SQL "+strings.TrimSpace(fmt.Sprintf(msg, data...)))
	}
}

// Trace 按 GORM 日志级别输出执行失败与慢 SQL；记录不存在属正常流，跳过；
// SQL 由 ParamsFilter 去掉绑定参数，避免敏感值落盘
func (l *gormLog) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.level >= gormlogger.Error:
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		sql, rows := fc()
		logger.Error(ctx, "SQL 执行失败",
			"sql", sql,
			"rows", rows,
			"elapsed_ms", elapsed.Milliseconds(),
			"error", err,
		)
	case l.slowThreshold > 0 && elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		sql, rows := fc()
		logger.Warn(ctx, "慢 SQL",
			"sql", sql,
			"rows", rows,
			"elapsed_ms", elapsed.Milliseconds(),
			"threshold_ms", l.slowThreshold.Milliseconds(),
		)
	case l.level >= gormlogger.Info:
		sql, rows := fc()
		logger.Info(ctx, "SQL",
			"sql", sql,
			"rows", rows,
			"elapsed_ms", elapsed.Milliseconds(),
		)
	}
}

// ParamsFilter 返回不含绑定参数的 SQL，供 GORM 生成日志文本
func (l *gormLog) ParamsFilter(_ context.Context, sql string, _ ...any) (string, []any) {
	return sql, nil
}
