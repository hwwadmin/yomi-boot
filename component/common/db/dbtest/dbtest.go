// Package dbtest 提供基于 go-mysql-mocker 的 MySQL 测试辅助
//
// @author zwei
package dbtest

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	gmm "github.com/hedon954/go-mysql-mocker/gmm"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	_ "github.com/go-sql-driver/mysql"
)

// config 内存 MySQL 实例的配置参数
type config struct {
	dbName   string
	port     int
	tables   []schema.Tabler
	data     []any
	sqlStmts []string
	sqlFiles []string
}

// Option 配置内存 MySQL 实例
type Option func(*config)

// WithDBName 指定数据库名，默认由 go-mysql-mocker 随机生成
func WithDBName(name string) Option {
	return func(c *config) {
		c.dbName = name
	}
}

// WithPort 指定监听端口，默认自动分配空闲端口
func WithPort(port int) Option {
	return func(c *config) {
		c.port = port
	}
}

// WithTables 按 GORM 模型自动建表
func WithTables(tables ...schema.Tabler) Option {
	return func(c *config) {
		c.tables = append(c.tables, tables...)
	}
}

// WithData 初始化测试数据，支持单条记录或切片
func WithData(data ...any) Option {
	return func(c *config) {
		c.data = append(c.data, data...)
	}
}

// WithSQL 执行初始化 SQL 语句
func WithSQL(stmts ...string) Option {
	return func(c *config) {
		c.sqlStmts = append(c.sqlStmts, stmts...)
	}
}

// WithSQLFiles 从 SQL 文件初始化 schema 或数据
func WithSQLFiles(files ...string) Option {
	return func(c *config) {
		c.sqlFiles = append(c.sqlFiles, files...)
	}
}

// New 启动内存 MySQL 并返回 GORM 客户端与清理函数
func New(t testing.TB, opts ...Option) (*gorm.DB, func()) {
	t.Helper()

	_, gormDB, cleanup := NewSQL(t, opts...)
	return gormDB, cleanup
}

// StartInfo 启动内存 MySQL 并返回监听端口、数据库名与关闭函数，
// 供需要原始连接信息的使用方（如 db.Open 集成测试）自行建立连接
func StartInfo(t testing.TB, opts ...Option) (port int, dbName string, shutdown func()) {
	t.Helper()

	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	builder, dbName := newBuilder(cfg)
	_, _, shutdown, err := builder.Build()
	if err != nil {
		t.Fatalf("dbtest: 启动 mock MySQL 失败: %v", err)
	}
	return builder.GetPort(), dbName, shutdown
}

// NewSQL 启动内存 MySQL 并返回 database/sql、GORM 客户端与清理函数
func NewSQL(t testing.TB, opts ...Option) (*sql.DB, *gorm.DB, func()) {
	t.Helper()

	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	builder, dbName := newBuilder(cfg)
	_, _, shutdown, err := builder.Build()
	if err != nil {
		t.Fatalf("dbtest: 启动 mock MySQL 失败: %v", err)
	}

	sqlDB, gormDB, err := openClient(builder.GetPort(), dbName)
	if err != nil {
		shutdown()
		t.Fatalf("dbtest: 连接 mock MySQL 失败: %v", err)
	}

	cleanup := func() {
		shutdown()
		_ = sqlDB.Close()
	}
	return sqlDB, gormDB, cleanup
}

// newBuilder 根据配置构建 GMM Builder，返回 builder 与数据库名称
func newBuilder(cfg *config) (*gmm.GMMBuilder, string) {
	dbName := cfg.dbName
	if dbName == "" {
		dbName = "yomi_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	}

	builder := gmm.Builder(dbName)
	if cfg.port > 0 {
		builder = builder.Port(cfg.port)
	}
	for _, table := range cfg.tables {
		builder = builder.CreateTable(table)
	}
	for _, data := range cfg.data {
		builder = builder.InitData(data)
	}
	if len(cfg.sqlStmts) > 0 {
		builder = builder.SQLStmts(cfg.sqlStmts...)
	}
	if len(cfg.sqlFiles) > 0 {
		builder = builder.SQLFiles(cfg.sqlFiles...)
	}
	return builder, dbName
}

// openClient 通过指定端口和数据库名称打开 MySQL 连接，返回 sql.DB 和 gorm.DB
func openClient(port int, dbName string) (*sql.DB, *gorm.DB, error) {
	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%d)/%s?parseTime=true&loc=Local", port, dbName)
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, err
	}
	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	return sqlDB, gormDB, nil
}
