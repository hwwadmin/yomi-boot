// 数据源连接集成与 Fail Fast 初始化测试
//
// @author zwei

package db

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// mockMySQLConfig 构造指向内存 mock MySQL 的数据源配置
func mockMySQLConfig(port int, dbName string, mutate func(*Config)) Config {
	zero := 0
	cfg := Config{
		Type:                   enums.MySQL,
		Host:                   "127.0.0.1",
		Port:                   port,
		User:                   "root",
		Database:               dbName,
		MaxOpenConns:           5,
		MaxIdleConns:           3,
		ConnMaxLifetimeMinutes: 30,
		ConnMaxIdleTimeMinutes: 30,
		SlowThresholdMs:        &zero,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

// TestOpen_MySQLMock 验证真实建连、读写与关闭契约
func TestOpen_MySQLMock(t *testing.T) {
	port, dbName, shutdown := dbtest.StartInfo(t, dbtest.WithTables(idleTestModel{}))
	defer shutdown()

	conn, err := Open(context.Background(), mockMySQLConfig(port, dbName, nil))
	if err != nil {
		t.Fatalf("打开数据源失败: %v", err)
	}
	if conn.GORM() == nil {
		t.Fatal("打开后 GORM 句柄不应为 nil")
	}
	if got := conn.Type(); got != enums.MySQL {
		t.Fatalf("Type 期望 mysql，实际 %s", got)
	}

	row := idleTestModel{ID: 1, Name: "a"}
	if err := conn.GORM().Create(&row).Error; err != nil {
		t.Fatalf("写入数据失败: %v", err)
	}
	var got idleTestModel
	if err := conn.GORM().First(&got, 1).Error; err != nil {
		t.Fatalf("查询数据失败: %v", err)
	}
	if got.Name != "a" {
		t.Fatalf("查询结果不符: %+v", got)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("关闭数据源失败: %v", err)
	}
	if conn.GORM() != nil {
		t.Fatal("关闭后 GORM 句柄应返回 nil")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("重复关闭应幂等: %v", err)
	}
}

// TestOpen_WithIdleKeeper 验证预热与最小空闲保活随连接启动、随关闭停止
func TestOpen_WithIdleKeeper(t *testing.T) {
	port, dbName, shutdown := dbtest.StartInfo(t, dbtest.WithTables(idleTestModel{}))
	defer shutdown()

	conn, err := Open(context.Background(), mockMySQLConfig(port, dbName, func(c *Config) {
		c.MinIdleConns = 2
	}))
	if err != nil {
		t.Fatalf("打开数据源失败: %v", err)
	}
	if conn.GORM() == nil {
		t.Fatal("打开后 GORM 句柄不应为 nil")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("关闭数据源失败: %v", err)
	}
}

// TestOpen_UnsupportedType 验证未支持的数据库类型在建连前报错
func TestOpen_UnsupportedType(t *testing.T) {
	cfg := mockMySQLConfig(1, "db", nil)
	cfg.Type = "oracle"
	if _, err := Open(context.Background(), cfg); err == nil {
		t.Fatal("未支持的数据库类型应返回错误")
	}
}

// TestMustOpen_MySQLMock 验证 Fail Fast 初始化成功路径
func TestMustOpen_MySQLMock(t *testing.T) {
	port, dbName, shutdown := dbtest.StartInfo(t, dbtest.WithTables(idleTestModel{}))
	defer shutdown()

	conn := MustOpen(context.Background(), mockMySQLConfig(port, dbName, nil))
	if conn == nil || conn.GORM() == nil {
		t.Fatal("MustOpen 应返回可用数据源")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("关闭数据源失败: %v", err)
	}
}

// TestMustOpen_PanicsOnInvalidConfig 验证配置非法时 panic
func TestMustOpen_PanicsOnInvalidConfig(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("配置非法应 panic")
		}
	}()
	MustOpen(context.Background(), Config{})
}

// TestConfigValidate 验证配置校验的各错误分支
func TestConfigValidate(t *testing.T) {
	valid := mockMySQLConfig(3306, "db", nil)
	if err := valid.Validate(); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "host 为空", mutate: func(c *Config) { c.Host = " " }},
		{name: "port 非正", mutate: func(c *Config) { c.Port = 0 }},
		{name: "user 为空", mutate: func(c *Config) { c.User = "" }},
		{name: "database 为空", mutate: func(c *Config) { c.Database = "" }},
		{name: "type 不支持", mutate: func(c *Config) { c.Type = "oracle" }},
		{name: "max_open_conns 非正", mutate: func(c *Config) { c.MaxOpenConns = 0 }},
		{name: "max_idle_conns 非正", mutate: func(c *Config) { c.MaxIdleConns = 0 }},
		{name: "min_idle_conns 为负", mutate: func(c *Config) { c.MinIdleConns = -1 }},
		{name: "min_idle_conns 超过 max_idle_conns", mutate: func(c *Config) { c.MinIdleConns = c.MaxIdleConns + 1 }},
		{name: "min_idle_conns 超过 max_open_conns", mutate: func(c *Config) { c.MinIdleConns = c.MaxOpenConns + 1; c.MaxIdleConns = c.MaxOpenConns + 2 }},
		{name: "lifetime 非正", mutate: func(c *Config) { c.ConnMaxLifetimeMinutes = 0 }},
		{name: "idle time 非正", mutate: func(c *Config) { c.ConnMaxIdleTimeMinutes = 0 }},
		{name: "slow_threshold_ms 为负", mutate: func(c *Config) { neg := -1; c.SlowThresholdMs = &neg }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			invalid := valid
			tc.mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatalf("%s 应返回错误", tc.name)
			}
		})
	}
}

// TestIsDuplicateKey 验证按数据库类型识别唯一键冲突
func TestIsDuplicateKey(t *testing.T) {
	cases := []struct {
		name   string
		dbType enums.DbType
		err    error
		want   bool
	}{
		{name: "gorm 统一错误", dbType: enums.MySQL, err: gorm.ErrDuplicatedKey, want: true},
		{name: "mysql 1062", dbType: enums.MySQL, err: &mysql.MySQLError{Number: 1062}, want: true},
		{name: "mysql 其他错误", dbType: enums.MySQL, err: &mysql.MySQLError{Number: 1045}, want: false},
		{name: "postgres 23505", dbType: enums.Postgres, err: &pgconn.PgError{Code: "23505"}, want: true},
		{name: "postgres 其他错误", dbType: enums.Postgres, err: &pgconn.PgError{Code: "42P01"}, want: false},
		{name: "普通错误", dbType: enums.MySQL, err: errors.New("boom"), want: false},
		{name: "nil 错误", dbType: enums.MySQL, err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDuplicateKey(tc.err, tc.dbType); got != tc.want {
				t.Fatalf("IsDuplicateKey 期望 %v，实际 %v", tc.want, got)
			}
		})
	}
}

// TestGormLog_InfoWarnError 验证日志级别门控与输出不 panic
func TestGormLog_InfoWarnError(t *testing.T) {
	ctx := context.Background()
	l := newGormLogger(Config{})

	// 默认 Warn 级别：Info 被门控跳过，Warn/Error 输出
	l.Info(ctx, "info %s", "x")
	l.Warn(ctx, "warn %s", "x")
	l.Error(ctx, "error %s", "x")

	// 提升到 Info 后 Info 输出
	info := l.LogMode(gormlogger.Info)
	info.Info(ctx, "info %s", "x")
	info.Warn(ctx, "warn %s", "x")
	info.Error(ctx, "error %s", "x")

	// Silent 级别全部跳过
	silent := l.LogMode(gormlogger.Silent)
	silent.Info(ctx, "info %s", "x")
	silent.Warn(ctx, "warn %s", "x")
	silent.Error(ctx, "error %s", "x")
}

// TestIdleKeeperLoop 维持最小空闲连接并在停止后退出
func TestIdleKeeperLoop(t *testing.T) {
	sqlDB, _, cleanup := dbtest.NewSQL(t, dbtest.WithTables(idleTestModel{}))
	defer cleanup()
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(3)

	k := startIdleKeeper(sqlDB, 2, 3, 20*time.Millisecond)
	if k == nil {
		t.Fatal("keeper 应启动")
	}
	// 等待若干轮巡检后空闲连接应补到下限
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sqlDB.Stats().Idle >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := sqlDB.Stats().Idle; got < 2 {
		t.Fatalf("保活应将空闲连接补到 2，实际 %d", got)
	}
	k.Stop()
	k.Stop() // 幂等
}

// TestIdleKeeperLoopOnClosedDB 验证关闭数据源后巡检失败只记日志不 panic
func TestIdleKeeperLoopOnClosedDB(t *testing.T) {
	sqlDB, _, cleanup := dbtest.NewSQL(t, dbtest.WithTables(idleTestModel{}))
	k := startIdleKeeper(sqlDB, 2, 3, 10*time.Millisecond)
	cleanup()
	// 关闭后巡检失败应只记日志，Stop 正常返回
	time.Sleep(50 * time.Millisecond)
	k.Stop()
}
