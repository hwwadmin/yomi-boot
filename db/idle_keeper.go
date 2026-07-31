// Package db 最小空闲连接预热与保活
//
// @author zwei
package db

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"
)

// idleWarmTimeout 启动预热超时
const idleWarmTimeout = 30 * time.Second

// idleKeeper 在后台维持最小空闲连接数（Go database/sql 无原生 min idle）
type idleKeeper struct {
	db       *sql.DB
	minIdle  int
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
	once     sync.Once
}

// maintainInterval 按空闲超时推算巡检间隔，避免空闲被回收后长时间低于下限
func maintainInterval(idleTimeout time.Duration) time.Duration {
	half := idleTimeout / 2
	const minInterval = 5 * time.Second
	const maxInterval = time.Minute
	if half < minInterval {
		return minInterval
	}
	if half > maxInterval {
		return maxInterval
	}
	return half
}

// warmIdle 同时占用 n 条连接并 Ping，释放后池中至少留下 n 条空闲（受 max_open 约束）
func warmIdle(ctx context.Context, sqlDB *sql.DB, n int) error {
	if n <= 0 {
		return nil
	}
	conns := make([]*sql.Conn, 0, n)
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	for i := 0; i < n; i++ {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return errors.Wrap(err, "预热数据库连接失败")
		}
		if err := conn.PingContext(ctx); err != nil {
			_ = conn.Close()
			return errors.Wrap(err, "预热数据库连接 Ping 失败")
		}
		conns = append(conns, conn)
	}
	return nil
}

// ensureMinIdle 在容量允许时把空闲连接补到 minIdle
func ensureMinIdle(ctx context.Context, sqlDB *sql.DB, minIdle int) error {
	if minIdle <= 0 {
		return nil
	}
	stats := sqlDB.Stats()
	if stats.Idle >= minIdle {
		return nil
	}
	return warmIdle(ctx, sqlDB, minIdle)
}

// startIdleKeeper 启动最小空闲保活；返回 stop，可重复调用且幂等
func startIdleKeeper(sqlDB *sql.DB, minIdle int, interval time.Duration) *idleKeeper {
	if minIdle <= 0 || sqlDB == nil {
		return nil
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	k := &idleKeeper{
		db:       sqlDB,
		minIdle:  minIdle,
		interval: interval,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	go k.loop(ctx)
	return k
}

func (k *idleKeeper) loop(ctx context.Context) {
	defer close(k.done)
	ticker := time.NewTicker(k.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cctx, cancel := context.WithTimeout(ctx, idleWarmTimeout)
			err := ensureMinIdle(cctx, k.db, k.minIdle)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.WarnWithoutCtx("维持最小空闲连接失败", "min_idle", k.minIdle, "error", err)
			}
		}
	}
}

// Stop 停止保活协程，可重复调用
func (k *idleKeeper) Stop() {
	if k == nil {
		return
	}
	k.once.Do(func() {
		k.cancel()
		<-k.done
	})
}
