// Package repo 的发件箱死信仓储测试
//
// @author zwei
package repo_test

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
)

// TestOutboxDlqRepoInsertUpdateDelete 验证死信的插入、全量更新与硬删除
func TestOutboxDlqRepoInsertUpdateDelete(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutboxDlq{}))
	defer cleanup()

	r := repo.NewOutboxDlqRepo(gormDB)
	lastTime := time.Now().Add(-time.Minute).Truncate(time.Second)
	afterTime := time.Now().Add(10 * time.Minute).Truncate(time.Second)
	row := model.SysEventOutboxDlq{
		ServiceName:    "svc",
		EventOutboxID:  10,
		ErrorMsg:       "boom",
		RetryCount:     1,
		RetryLastTime:  &lastTime,
		RetryAfterTime: &afterTime,
	}
	if err := r.Insert(context.Background(), &row); err != nil {
		t.Fatalf("插入死信失败: %v", err)
	}
	if row.ID == 0 {
		t.Fatalf("插入后主键应回填: %d", row.ID)
	}

	var stored model.SysEventOutboxDlq
	if err := gormDB.First(&stored, row.ID).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if stored.ErrorMsg != "boom" || stored.RetryCount != 1 || stored.EventOutboxID != 10 {
		t.Fatalf("插入字段不符: %+v", stored)
	}

	stored.ErrorMsg = "boom-2"
	stored.RetryCount = 2
	if err := r.Update(context.Background(), &stored); err != nil {
		t.Fatalf("全量更新死信失败: %v", err)
	}
	var updated model.SysEventOutboxDlq
	if err := gormDB.First(&updated, row.ID).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if updated.ErrorMsg != "boom-2" || updated.RetryCount != 2 {
		t.Fatalf("全量更新字段未生效: %+v", updated)
	}

	if err := r.Delete(context.Background(), row.ID); err != nil {
		t.Fatalf("删除死信失败: %v", err)
	}
	var all []model.SysEventOutboxDlq
	if err := gormDB.Unscoped().Find(&all).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("期望硬删除后无残留记录，实际 %d 条", len(all))
	}
}

// TestOutboxDlqRepoFetch 验证只取未超最大重试且已过退避窗口的死信，并按可重投时间升序限量
func TestOutboxDlqRepoFetch(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutboxDlq{}))
	defer cleanup()

	base := time.Now().Add(-time.Hour)
	now := base.Add(30 * time.Minute)
	past := base.Add(10 * time.Minute)
	future := base.Add(40 * time.Minute)
	rows := []model.SysEventOutboxDlq{
		{ServiceName: "svc", EventOutboxID: 6, RetryCount: 0},
		{ServiceName: "svc", EventOutboxID: 2, RetryCount: 3},
		{ServiceName: "svc", EventOutboxID: 3, RetryCount: 0, RetryAfterTime: &future},
		{ServiceName: "svc", EventOutboxID: 1, RetryCount: 1},
		{ServiceName: "svc", EventOutboxID: 4, RetryCount: 0, RetryAfterTime: &past},
		{ServiceName: "other", EventOutboxID: 5},
	}
	createTimes := []time.Time{
		base.Add(time.Minute),
		base.Add(2 * time.Minute),
		base.Add(3 * time.Minute),
		base.Add(5 * time.Minute),
		base.Add(4 * time.Minute),
		base,
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
		rows[i].CreateTime = createTimes[i]
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化死信失败: %v", err)
	}

	r := repo.NewOutboxDlqRepo(gormDB)
	got, err := r.Fetch(context.Background(), "svc", 10, 3, now)
	if err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	// 排序键为 COALESCE(retry_after_time, create_time)：id6 与 id1 取 create_time，id4 取已过期的 retry_after_time
	if len(got) != 3 {
		t.Fatalf("期望 3 条可重试死信，实际 %d 条: %+v", len(got), got)
	}
	if got[0].EventOutboxID != 6 || got[1].EventOutboxID != 1 || got[2].EventOutboxID != 4 {
		t.Fatalf("期望按可重投时间升序返回 6/1/4，实际 %v", outboxDlqIDs(got))
	}

	got, err = r.Fetch(context.Background(), "svc", 2, 3, now)
	if err != nil {
		t.Fatalf("限量查询死信失败: %v", err)
	}
	if len(got) != 2 || got[0].EventOutboxID != 6 || got[1].EventOutboxID != 1 {
		t.Fatalf("限量 2 条期望返回 6/1，实际 %v", outboxDlqIDs(got))
	}

	got, err = r.Fetch(context.Background(), "svc", 10, 0, now)
	if err != nil || len(got) != 0 {
		t.Fatalf("maxRetry<=0 应返回空，实际 %v err=%v", outboxDlqIDs(got), err)
	}
}

// TestOutboxDlqRepoDeleteWithOutboxSuccess 验证回写发件箱成功并删除死信
func TestOutboxDlqRepoDeleteWithOutboxSuccess(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventOutbox{}, model.SysEventOutboxDlq{}),
	)
	defer cleanup()

	base := time.Now().Add(-time.Hour)
	outboxRepo := repo.NewOutboxRepo(gormDB)
	r := repo.NewOutboxDlqRepo(gormDB)
	ctx := context.Background()

	outbox := model.SysEventOutbox{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusSentFail}
	outbox.ID = 10
	dlq := model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 10, ErrorMsg: "boom"}
	dlq.ID = 20
	successTime := base.Add(time.Minute).Truncate(time.Second)
	if err := gormDB.Create(&outbox).Error; err != nil {
		t.Fatalf("初始化发件箱失败: %v", err)
	}
	if err := gormDB.Create(&dlq).Error; err != nil {
		t.Fatalf("初始化死信失败: %v", err)
	}
	if err := r.DeleteWithOutboxSuccess(ctx, &outbox, &dlq, successTime, outboxRepo); err != nil {
		t.Fatalf("死信重投成功回写失败: %v", err)
	}
	if outbox.Status != model.OutboxStatusSentSuccess || outbox.SentTime == nil {
		t.Fatalf("入参发件箱应被置为发送成功: %+v", outbox)
	}
	var storedOutbox model.SysEventOutbox
	if err := gormDB.First(&storedOutbox, 10).Error; err != nil {
		t.Fatalf("查询发件箱失败: %v", err)
	}
	if storedOutbox.Status != model.OutboxStatusSentSuccess || storedOutbox.SentTime == nil {
		t.Fatalf("发件箱应被标记为发送成功: %+v", storedOutbox)
	}
	var dlqCount int64
	if err := gormDB.Unscoped().Model(&model.SysEventOutboxDlq{}).Where("id = ?", 20).Count(&dlqCount).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if dlqCount != 0 {
		t.Fatalf("死信应被硬删除，实际 %d 条", dlqCount)
	}

	// 已是发送成功的发件箱不应被覆盖原发送时间，但死信仍要删除
	already := model.SysEventOutbox{
		ServiceName: "svc", EventID: "event-2", Topic: "topic",
		Status: model.OutboxStatusSentSuccess, SentTime: &successTime,
	}
	already.ID = 11
	dlq2 := model.SysEventOutboxDlq{ServiceName: "svc", EventOutboxID: 11, ErrorMsg: "stale"}
	dlq2.ID = 21
	if err := gormDB.Create(&already).Error; err != nil {
		t.Fatalf("初始化发件箱失败: %v", err)
	}
	if err := gormDB.Create(&dlq2).Error; err != nil {
		t.Fatalf("初始化死信失败: %v", err)
	}
	later := successTime.Add(10 * time.Minute)
	if err := r.DeleteWithOutboxSuccess(ctx, &already, &dlq2, later, outboxRepo); err != nil {
		t.Fatalf("已成功记录的死信清理失败: %v", err)
	}
	var storedAlready model.SysEventOutbox
	if err := gormDB.First(&storedAlready, 11).Error; err != nil {
		t.Fatalf("查询发件箱失败: %v", err)
	}
	if storedAlready.SentTime == nil || !storedAlready.SentTime.Equal(successTime) {
		t.Fatalf("原发送时间不应被覆盖，期望 %v 实际 %v", successTime, storedAlready.SentTime)
	}
	var staleCount int64
	if err := gormDB.Unscoped().Model(&model.SysEventOutboxDlq{}).Where("id = ?", 21).Count(&staleCount).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if staleCount != 0 {
		t.Fatalf("过期死信应被硬删除，实际 %d 条", staleCount)
	}
}

// TestOutboxDlqRepoResetRetryCountInTimeRange 验证按创建时间范围清零重试计数
func TestOutboxDlqRepoResetRetryCountInTimeRange(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutboxDlq{}))
	defer cleanup()

	base := time.Now().Add(-time.Hour)
	lastTime := base.Add(time.Minute)
	afterTime := base.Add(10 * time.Minute)
	rows := []model.SysEventOutboxDlq{
		{ServiceName: "svc", EventOutboxID: 1, RetryCount: 3, RetryLastTime: &lastTime, RetryAfterTime: &afterTime},
		{ServiceName: "svc", EventOutboxID: 2, RetryCount: 2, RetryLastTime: &lastTime, RetryAfterTime: &afterTime},
		{ServiceName: "other", EventOutboxID: 3, RetryCount: 1, RetryLastTime: &lastTime, RetryAfterTime: &afterTime},
	}
	createTimes := []time.Time{base.Add(time.Minute), base.Add(30 * time.Minute), base.Add(time.Minute)}
	for i := range rows {
		rows[i].ID = int64(i + 1)
		rows[i].CreateTime = createTimes[i]
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化死信失败: %v", err)
	}

	r := repo.NewOutboxDlqRepo(gormDB)
	affected, err := r.ResetRetryCountInTimeRange(
		context.Background(), "svc", base, base.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatalf("重置重试计数失败: %v", err)
	}
	if affected != 1 {
		t.Fatalf("期望重置 1 条，实际 %d 条", affected)
	}

	var stored []model.SysEventOutboxDlq
	if err := gormDB.Order("id ASC").Find(&stored).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if stored[0].RetryCount != 0 || stored[0].RetryLastTime != nil || stored[0].RetryAfterTime != nil {
		t.Fatalf("范围内记录的重试字段应被清零: %+v", stored[0])
	}
	if stored[1].RetryCount != 2 || stored[1].RetryLastTime == nil || stored[1].RetryAfterTime == nil {
		t.Fatalf("范围外记录不应被重置: %+v", stored[1])
	}
	if stored[2].RetryCount != 1 {
		t.Fatalf("其他服务记录不应被重置: %+v", stored[2])
	}

	affected, err = r.ResetRetryCountInTimeRange(
		context.Background(), "svc", base.Add(10*time.Minute), base,
	)
	if err == nil || affected != 0 {
		t.Fatalf("结束时间早于开始时间应报错，实际 affected=%d err=%v", affected, err)
	}
}

// outboxDlqIDs 提取死信记录的发件箱 ID 列表，便于断言输出
func outboxDlqIDs(rows []model.SysEventOutboxDlq) []int64 {
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].EventOutboxID)
	}
	return ids
}
