// Package db 最小空闲连接预热与保活
//
// @author zwei
package db

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

// idleWarmTimeout 启动预热超时
const idleWarmTimeout = 30 * time.Second

// idleKeepTimeout 保活补连接的等待上限，避免与业务抢连接时长时间阻塞
const idleKeepTimeout = 2 * time.Second

// idleKeeper 在后台维持最小空闲连接数（Go database/sql 无原生 min idle）
type idleKeeper struct {
	db       *sql.DB
	minIdle  int
	maxIdle  int
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

// warmIdle 同时占用 n 条连接并 Ping，释放后池中空闲数提升到 n（受 max_idle 约束）
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

// ensureMinIdle 在容量允许时把空闲连接补到 minIdle。
// 抬高空闲数必须同时持有 minIdle 条连接（复用已有空闲连接也算占用），
// 因此按 max_idle 与池剩余容量裁剪持有量；无提升空间时直接跳过，不阻塞业务。
func ensureMinIdle(ctx context.Context, sqlDB *sql.DB, minIdle, maxIdle int) error {
	if minIdle <= 0 {
		return nil
	}
	stats := sqlDB.Stats()
	if stats.Idle >= minIdle {
		return nil
	}

	hold := minIdle
	if maxIdle > 0 && hold > maxIdle {
		hold = maxIdle
	}
	if stats.MaxOpenConnections > 0 {
		// max_open - in_use 包含当前空闲连接与可新开的空位，持有量不超过它才不会等待连接
		if avail := stats.MaxOpenConnections - stats.InUse; hold > avail {
			hold = avail
		}
	}
	// 持有量不超过当前空闲数时只会复用已有空闲连接，无法提升，跳过本轮
	if hold <= stats.Idle {
		return nil
	}
	return warmIdle(ctx, sqlDB, hold)
}

// startIdleKeeper 启动最小空闲保活；返回 stop，可重复调用且幂等
func startIdleKeeper(sqlDB *sql.DB, minIdle, maxIdle int, interval time.Duration) *idleKeeper {
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
		maxIdle:  maxIdle,
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
			cctx, cancel := context.WithTimeout(ctx, idleKeepTimeout)
			err := ensureMinIdle(cctx, k.db, k.minIdle, k.maxIdle)
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
