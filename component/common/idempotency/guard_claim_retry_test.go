// 业务幂等守卫 INSERT 冲突回退循环的边界测试：反查失败传播与有限重抢
//
// @author zwei

package idempotency

import (
	"context"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/errors"

	"gorm.io/gorm"
)

// findProbeCtxKey 是冲突回退用例的同步标记键：仅带该标记的查询/插入会触发回调干预，
// 把「冲突的抢占行刚被回滚」等时序窗口固定成确定性步骤
type findProbeCtxKey struct{}

// dropRowBeforeFind 注册查询回调：带标记 ctx 的查询执行前先物理删除指定行，
// 等价于冲突的抢占行在反查前消失（回滚），反查无记录后走到有限重抢
func dropRowBeforeFind(t *testing.T, db *gorm.DB, id int64) {
	t.Helper()
	if err := db.Callback().Query().Before("gorm:query").Register("test:drop_conflict_row", func(gdb *gorm.DB) {
		if gdb.Statement == nil || gdb.Statement.Context.Value(findProbeCtxKey{}) == nil {
			return
		}
		if err := db.Session(&gorm.Session{NewDB: true}).Exec("DELETE FROM t_sys_biz WHERE id = ?", id).Error; err != nil {
			// 累加进当前查询，由后续 Error 读出；返回值为累计错误，此处刻意不取
			_ = gdb.AddError(err)
		}
	}); err != nil {
		t.Fatalf("注册查询回调失败: %v", err)
	}
}

// TestRunBusinessOnce_ConflictRegrabSucceedsAfterRowVanished 抢占冲突且反查无记录（冲突行刚被回滚）
// 时第二次重抢成功走出循环的正常语义：业务执行一次并以已完成状态落库
func TestRunBusinessOnce_ConflictRegrabSucceedsAfterRowVanished(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	// 预置一条提交态的同键抢占行，制造首次 TryInsert 的真实唯一键冲突
	repo := NewRepository(env.db)
	const conflictID = int64(940)
	if claimed, err := repo.TryInsert(ctx, &SysBiz{
		ID:    conflictID,
		BizID: "biz-regrab", BizSubID: "sub-regrab", OperationType: "TEST_OP", Status: StatusProcessing,
	}); err != nil || !claimed {
		t.Fatalf("造数失败, claimed=%v, err=%v", claimed, err)
	}
	dropRowBeforeFind(t, env.db, conflictID)

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "regrab-result", nil
	}
	snap, err := runBusinessOnce[string](env.guard, context.WithValue(ctx, findProbeCtxKey{}, true), testKey("regrab"), fn)
	if err != nil {
		t.Fatalf("重抢成功后应正常执行业务: %v", err)
	}
	if snap == nil || snap.result != "regrab-result" || calls != 1 {
		t.Fatalf("重抢成功应执行业务一次: snap=%v, calls=%d", snap, calls)
	}

	// 冲突行已消失，仅剩重抢成功的新行且已完成
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 1 {
		t.Fatalf("应只保留重抢成功的记录, count=%d", count)
	}
	var row SysBiz
	if err = env.db.Where("biz_id = ? AND biz_sub_id = ?", "biz-regrab", "sub-regrab").First(&row).Error; err != nil {
		t.Fatalf("查询幂等记录失败: %v", err)
	}
	if row.Status != StatusCompleted || row.ID == conflictID {
		t.Fatalf("重抢的新行应已完成, id=%d, status=%d", row.ID, row.Status)
	}
}

// TestRunBusinessOnce_ConflictFindProcessingPropagates 抢占冲突后反查命中处理中记录（防御场景）：
// findCompletedSnapshot 返回的 ErrProcessing 在冲突回退里直接传播，业务不执行
func TestRunBusinessOnce_ConflictFindProcessingPropagates(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	// 预置一条提交态的处理中记录：首次抢占冲突、循环内反查得到非完成状态
	repo := NewRepository(env.db)
	if claimed, err := repo.TryInsert(ctx, &SysBiz{
		ID:    950,
		BizID: "biz-find-err", BizSubID: "sub-find-err", OperationType: "TEST_OP", Status: StatusProcessing,
	}); err != nil || !claimed {
		t.Fatalf("造数失败, claimed=%v, err=%v", claimed, err)
	}

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	_, err := runBusinessOnce[string](env.guard, ctx, testKey("find-err"), fn)
	if !errors.Is(err, ErrProcessing) {
		t.Fatalf("反查到处理中记录应返回 ErrProcessing, err=%v", err)
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 1 {
		t.Fatalf("冲突回退失败不应新增幂等记录, count=%d", count)
	}
}

// TestRunBusinessOnce_ConflictFindDBErrorPropagates 抢占冲突后反查真实失败（查询 ctx 被取消）：
// FindByKey 的 DB 错误经 findCompletedSnapshot 直接传播，业务不执行
func TestRunBusinessOnce_ConflictFindDBErrorPropagates(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// 预置一条提交态同键抢占行：首次 TryInsert 真实冲突返回 (false, nil)
	repo := NewRepository(env.db)
	if claimed, err := repo.TryInsert(context.Background(), &SysBiz{
		ID:    960,
		BizID: "biz-find-db-err", BizSubID: "sub-find-db-err", OperationType: "TEST_OP", Status: StatusProcessing,
	}); err != nil || !claimed {
		t.Fatalf("造数失败, claimed=%v, err=%v", claimed, err)
	}

	// 查询回调：带标记 ctx 的反查执行前取消 ctx，让 FindByKey 的 SELECT 真实失败
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runCtx = context.WithValue(runCtx, findProbeCtxKey{}, true)
	if err := env.db.Callback().Query().Before("gorm:query").Register("test:cancel_find", func(gdb *gorm.DB) {
		if gdb.Statement == nil || gdb.Statement.Context.Value(findProbeCtxKey{}) == nil {
			return
		}
		cancel()
	}); err != nil {
		t.Fatalf("注册查询回调失败: %v", err)
	}

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	_, err := runBusinessOnce[string](env.guard, runCtx, testKey("find-db-err"), fn)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("反查 DB 失败应透传错误, err=%v", err)
	}
	if errors.Is(err, ErrDuplicateWithoutRecord) {
		t.Fatalf("反查失败应直接返回而非继续重抢, err=%v", err)
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
}

// TestRunBusinessOnce_RegrabDBErrorPropagates 反查无记录后第二次重抢真实失败（重抢前 ctx 被取消）：
// 重抢的错误在冲突回退里直接传播，业务不执行、无残留记录
func TestRunBusinessOnce_RegrabDBErrorPropagates(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	// 预置一条提交态同键抢占行：首次 TryInsert 真实冲突返回 (false, nil)
	repo := NewRepository(env.db)
	const conflictID = int64(970)
	if claimed, err := repo.TryInsert(context.Background(), &SysBiz{
		ID:    conflictID,
		BizID: "biz-regrab-db-err", BizSubID: "sub-regrab-db-err", OperationType: "TEST_OP", Status: StatusProcessing,
	}); err != nil || !claimed {
		t.Fatalf("造数失败, claimed=%v, err=%v", claimed, err)
	}
	dropRowBeforeFind(t, env.db, conflictID)

	// 查询回调：反查执行后取消 ctx，让第二次重抢的 BEGIN 真实失败（不污染反查本身）
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runCtx = context.WithValue(runCtx, findProbeCtxKey{}, true)
	if err := env.db.Callback().Query().After("gorm:query").Register("test:cancel_regrab", func(gdb *gorm.DB) {
		if gdb.Statement == nil || gdb.Statement.Context.Value(findProbeCtxKey{}) == nil {
			return
		}
		cancel()
	}); err != nil {
		t.Fatalf("注册查询回调失败: %v", err)
	}

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	_, err := runBusinessOnce[string](env.guard, runCtx, testKey("regrab-db-err"), fn)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("第二次重抢失败应透传错误, err=%v", err)
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 0 {
		t.Fatalf("重抢失败不应残留幂等记录, count=%d", count)
	}
}
