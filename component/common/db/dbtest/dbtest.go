// Package dbtest 提供基于 Testcontainers 的 MySQL 8.4 测试辅助。
//
// 每个测试进程共用一个 mysql:8.4.11 容器，每个用例使用独立数据库，用例结束时删除该库。
// 容器在测试进程退出时由 Testcontainers 回收。
//
// @author zwei
package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	_ "github.com/go-sql-driver/mysql"
)

const (
	// Image 是测试容器使用的 MySQL 镜像，与 README 推荐的线上版本一致
	Image = "mysql:8.4.11"
	// Host 是测试代码连接容器时使用的地址。容器端口发布在本机。
	Host = "127.0.0.1"
	// User 是测试容器的登录用户
	User = "root"
	// Password 是测试容器的登录密码
	Password = "root"

	mysqlStartupTimeout = 3 * time.Minute
	schemaTimeout       = 30 * time.Second
)

// config 单个测试数据库的初始化参数
type config struct {
	dbName   string
	tables   []schema.Tabler
	data     []any
	sqlStmts []string
	sqlFiles []string
}

// Option 配置单个测试数据库
type Option func(*config)

// WithDBName 指定数据库名。默认生成 yomi_ 前缀的随机名。
func WithDBName(name string) Option {
	return func(c *config) {
		c.dbName = name
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

// mysqlServer 是当前测试进程共用的 MySQL 容器
type mysqlServer struct {
	port  int
	admin *sql.DB
}

var (
	startOnce sync.Once
	server    *mysqlServer
	startErr  error
)

// New 启动测试数据库并返回 GORM 客户端与清理函数。
// 清理函数关闭连接并删除该用例的数据库，不停止容器。
func New(t testing.TB, opts ...Option) (*gorm.DB, func()) {
	t.Helper()

	_, gormDB, cleanup := NewSQL(t, opts...)
	return gormDB, cleanup
}

// StartInfo 启动测试数据库并返回监听端口、数据库名与清理函数，
// 供需要自行建立连接的使用方（如 db.Open 集成测试）使用。
// 连接地址为 Host，账号为 User / Password。
func StartInfo(t testing.TB, opts ...Option) (port int, dbName string, shutdown func()) {
	t.Helper()

	port, dbName, sqlDB, gormDB, cleanup := provision(t, opts...)
	closeClients(sqlDB, gormDB)
	return port, dbName, cleanup
}

// NewSQL 启动测试数据库并返回 database/sql、GORM 客户端与清理函数
func NewSQL(t testing.TB, opts ...Option) (*sql.DB, *gorm.DB, func()) {
	t.Helper()

	_, _, sqlDB, gormDB, cleanup := provision(t, opts...)
	return sqlDB, gormDB, cleanup
}

// provision 确保容器已启动，创建独立数据库并完成初始化
func provision(t testing.TB, opts ...Option) (port int, dbName string, sqlDB *sql.DB, gormDB *gorm.DB, cleanup func()) {
	t.Helper()

	srv := ensureServer(t)
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	dbName = cfg.dbName
	if dbName == "" {
		dbName = "yomi_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	}

	if err := srv.createDatabase(dbName); err != nil {
		t.Fatalf("dbtest: 创建数据库 %s 失败: %v", dbName, err)
	}

	var openErr error
	sqlDB, gormDB, openErr = openClient(srv.port, dbName)
	if openErr != nil {
		_ = srv.dropDatabase(dbName)
		t.Fatalf("dbtest: 连接 MySQL 失败: %v", openErr)
	}
	if err := applySchema(srv.port, dbName, gormDB, cfg); err != nil {
		closeClients(sqlDB, gormDB)
		_ = srv.dropDatabase(dbName)
		t.Fatalf("dbtest: 初始化数据库 %s 失败: %v", dbName, err)
	}

	var once sync.Once
	cleanup = func() {
		once.Do(func() {
			closeClients(sqlDB, gormDB)
			if err := srv.dropDatabase(dbName); err != nil {
				t.Errorf("dbtest: 删除数据库 %s 失败: %v", dbName, err)
			}
		})
	}
	return srv.port, dbName, sqlDB, gormDB, cleanup
}

// ensureServer 懒启动当前进程唯一的 MySQL 容器
func ensureServer(t testing.TB) *mysqlServer {
	t.Helper()

	startOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), mysqlStartupTimeout)
		defer cancel()

		ctr, err := mysql.Run(ctx, Image,
			withRootPassword(Password, "yomi_bootstrap"),
			testcontainers.WithCmd(
				"mysqld",
				"--character-set-server=utf8mb4",
				"--collation-server=utf8mb4_unicode_ci",
			),
			testcontainers.WithWaitStrategyAndDeadline(mysqlStartupTimeout,
				wait.ForLog("port: 3306  MySQL Community Server"),
			),
		)
		if err != nil {
			startErr = fmt.Errorf("启动 MySQL 容器: %w", err)
			return
		}

		mapped, err := ctr.MappedPort(ctx, "3306/tcp")
		if err != nil {
			startErr = fmt.Errorf("读取 MySQL 映射端口: %w", err)
			return
		}
		hostPort := int(mapped.Num())

		admin, err := sql.Open("mysql", dsn(hostPort, "mysql"))
		if err != nil {
			startErr = fmt.Errorf("打开管理连接: %w", err)
			return
		}
		admin.SetMaxOpenConns(8)
		admin.SetMaxIdleConns(2)

		pingCtx, pingCancel := context.WithTimeout(context.Background(), schemaTimeout)
		defer pingCancel()
		if err := pingReady(pingCtx, admin); err != nil {
			_ = admin.Close()
			startErr = fmt.Errorf("等待 MySQL 可连接: %w", err)
			return
		}
		server = &mysqlServer{port: hostPort, admin: admin}
	})
	if startErr != nil {
		t.Fatalf("dbtest: 启动 MySQL 容器失败: %v", startErr)
	}
	return server
}

// withRootPassword 使用 root 登录，并清掉模块默认的 MYSQL_USER。
// 官方镜像不允许只设置 MYSQL_PASSWORD 而不设置 MYSQL_USER；root 只认 MYSQL_ROOT_PASSWORD。
func withRootPassword(password, database string) testcontainers.CustomizeRequestOption {
	return func(req *testcontainers.GenericContainerRequest) error {
		delete(req.Env, "MYSQL_USER")
		delete(req.Env, "MYSQL_PASSWORD")
		req.Env["MYSQL_ROOT_PASSWORD"] = password
		req.Env["MYSQL_DATABASE"] = database
		return nil
	}
}

// pingReady 在容器日志报就绪后继续探测，避开初始化阶段的短暂重启
func pingReady(ctx context.Context, admin *sql.DB) error {
	var last error
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		last = admin.PingContext(pingCtx)
		cancel()
		if last == nil {
			return nil
		}
		if ctx.Err() != nil {
			return last
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last
		case <-timer.C:
		}
	}
}

func (s *mysqlServer) createDatabase(dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), schemaTimeout)
	defer cancel()
	stmt := fmt.Sprintf(
		"CREATE DATABASE %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		quoteIdent(dbName),
	)
	_, err := s.admin.ExecContext(ctx, stmt)
	return err
}

func (s *mysqlServer) dropDatabase(dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), schemaTimeout)
	defer cancel()
	_, err := s.admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quoteIdent(dbName))
	return err
}

// applySchema 按 WithTables、WithData、WithSQL、WithSQLFiles 的顺序初始化
func applySchema(port int, dbName string, gormDB *gorm.DB, cfg *config) error {
	if len(cfg.tables) > 0 {
		models := make([]any, len(cfg.tables))
		for i, table := range cfg.tables {
			models[i] = table
		}
		if err := gormDB.AutoMigrate(models...); err != nil {
			return fmt.Errorf("建表: %w", err)
		}
	}
	for _, row := range cfg.data {
		if err := gormDB.Create(asPointer(row)).Error; err != nil {
			return fmt.Errorf("写入初始数据: %w", err)
		}
	}
	for _, stmt := range cfg.sqlStmts {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if err := gormDB.Exec(stmt).Error; err != nil {
			return fmt.Errorf("执行 SQL: %w", err)
		}
	}
	for _, path := range cfg.sqlFiles {
		if err := execSQLFile(port, dbName, path); err != nil {
			return err
		}
	}
	return nil
}

// execSQLFile 用 multiStatements 执行整个 SQL 文件
func execSQLFile(port int, dbName, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 SQL 文件 %s: %w", path, err)
	}
	script := strings.TrimSpace(string(content))
	if script == "" {
		return nil
	}

	db, err := sql.Open("mysql", dsn(port, dbName)+"&multiStatements=true")
	if err != nil {
		return fmt.Errorf("打开 SQL 文件连接 %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), schemaTimeout)
	defer cancel()
	if _, err := db.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("执行 SQL 文件 %s: %w", path, err)
	}
	return nil
}

// openClient 打开指向指定数据库的 sql.DB 与 GORM 客户端
func openClient(port int, dbName string) (*sql.DB, *gorm.DB, error) {
	dsn := dsn(port, dbName)
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, nil, err
	}
	gormDB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	return sqlDB, gormDB, nil
}

func dsn(port int, dbName string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local", User, Password, Host, port, dbName)
}

func closeClients(sqlDB *sql.DB, gormDB *gorm.DB) {
	if sqlDB != nil {
		_ = sqlDB.Close()
	}
	if gormDB == nil {
		return
	}
	raw, err := gormDB.DB()
	if err != nil {
		return
	}
	_ = raw.Close()
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// asPointer 保证 GORM Create 收到指针。传入指针或切片指针时原样返回。
func asPointer(v any) any {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() == reflect.Ptr {
		return v
	}
	ptr := reflect.New(rv.Type())
	ptr.Elem().Set(rv)
	return ptr.Interface()
}
