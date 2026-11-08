// dbtest 的 Postgres 测试容器辅助，与 dbtest.go 的 MySQL 入口平行的第二方言。
//
// 每个测试进程共用一个 postgres:16.15-alpine 容器，每个用例使用独立数据库，用例结束时删除该库。
// 容器在测试进程退出时由 Testcontainers 回收。
//
// @author zwei

package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	// PostgresImage 是测试容器使用的 PostgreSQL 镜像，与 README 推荐的线上版本一致
	PostgresImage = "postgres:16.15-alpine"
	// PostgresUser 是测试容器的登录用户
	PostgresUser = "root"
	// PostgresPassword 是测试容器的登录密码
	PostgresPassword = "root"

	postgresPort       = "5432/tcp"
	postgresDBTimeout  = 3 * time.Minute
	postgresSchemaName = "yomi_bootstrap"
)

// pgServer 是当前测试进程共用的 Postgres 容器
type pgServer struct {
	port  int
	admin *sql.DB
}

var (
	pgStartOnce sync.Once
	pgSrv       *pgServer
	pgStartErr  error
)

// PostgresStartInfo 启动测试 Postgres 并返回监听端口、数据库名与清理函数，
// 供需要自行建立连接的使用方（如 db.Open 的 Postgres 集成测试）使用。
// 连接地址为 Host，账号为 PostgresUser / PostgresPassword。
// 仅支持 WithDBName 指定库名；建表与数据初始化仍走 MySQL 入口（dbtest.New/NewSQL）。
func PostgresStartInfo(t testing.TB, opts ...Option) (port int, dbName string, shutdown func()) {
	t.Helper()

	srv := ensurePostgresServer(t)
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	if len(cfg.tables) > 0 || len(cfg.data) > 0 || len(cfg.sqlStmts) > 0 || len(cfg.sqlFiles) > 0 {
		t.Fatal("dbtest: Postgres 入口不支持建表与数据初始化选项，请改用 MySQL 入口")
	}
	dbName = cfg.dbName
	if dbName == "" {
		dbName = "yomi_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	}

	if err := srv.createDatabase(dbName); err != nil {
		t.Fatalf("dbtest: 创建 Postgres 数据库 %s 失败: %v", dbName, err)
	}

	var once sync.Once
	shutdown = func() {
		once.Do(func() {
			if err := srv.dropDatabase(dbName); err != nil {
				t.Errorf("dbtest: 删除 Postgres 数据库 %s 失败: %v", dbName, err)
			}
		})
	}
	return srv.port, dbName, shutdown
}

// ensurePostgresServer 懒启动当前进程唯一的 Postgres 容器
func ensurePostgresServer(t testing.TB) *pgServer {
	t.Helper()

	pgStartOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), postgresDBTimeout)
		defer cancel()

		// 官方镜像初始化时会两轮打印 ready 日志（临时实例 + 重启后的正式实例），以出现两次为准
		ctr, err := testcontainers.Run(ctx, PostgresImage,
			testcontainers.WithEnv(map[string]string{
				"POSTGRES_USER":     PostgresUser,
				"POSTGRES_PASSWORD": PostgresPassword,
				"POSTGRES_DB":       postgresSchemaName,
			}),
			testcontainers.WithWaitStrategyAndDeadline(postgresDBTimeout,
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			),
		)
		if err != nil {
			pgStartErr = fmt.Errorf("启动 Postgres 容器: %w", err)
			return
		}

		mapped, err := ctr.MappedPort(ctx, postgresPort)
		if err != nil {
			pgStartErr = fmt.Errorf("读取 Postgres 映射端口: %w", err)
			return
		}
		hostPort := int(mapped.Num())

		admin, err := sql.Open("pgx", pgAdminDSN(hostPort))
		if err != nil {
			pgStartErr = fmt.Errorf("打开 Postgres 管理连接: %w", err)
			return
		}
		admin.SetMaxOpenConns(2)
		admin.SetMaxIdleConns(1)

		pingCtx, pingCancel := context.WithTimeout(context.Background(), schemaTimeout)
		defer pingCancel()
		if err := pingReady(pingCtx, admin); err != nil {
			_ = admin.Close()
			pgStartErr = fmt.Errorf("等待 Postgres 可连接: %w", err)
			return
		}
		pgSrv = &pgServer{port: hostPort, admin: admin}
	})
	if pgStartErr != nil {
		t.Fatalf("dbtest: 启动 Postgres 容器失败: %v", pgStartErr)
	}
	return pgSrv
}

func pgAdminDSN(port int) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		Host, port, PostgresUser, PostgresPassword, postgresSchemaName)
}

func (s *pgServer) createDatabase(dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), schemaTimeout)
	defer cancel()
	_, err := s.admin.ExecContext(ctx, "CREATE DATABASE "+quotePGIdent(dbName))
	return err
}

func (s *pgServer) dropDatabase(dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), schemaTimeout)
	defer cancel()
	// WITH (FORCE) 断开残留连接后删库，避免用例连接未关干净时清理失败
	_, err := s.admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quotePGIdent(dbName)+" WITH (FORCE)")
	return err
}

func quotePGIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
