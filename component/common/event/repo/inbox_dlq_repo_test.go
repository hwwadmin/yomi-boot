// Package repo 的收件箱死信仓储测试
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

// TestInboxDlqRepoInsertIdempotent 验证同一服务下重复事件只保留首条死信
func TestInboxDlqRepoInsertIdempotent(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInboxDlq{}))
	defer cleanup()

	r := repo.NewInboxDlqRepo(gormDB)
	ctx := context.Background()
	first := &model.SysEventInboxDlq{
		ServiceName: "svc",
		EventID:     "event-1",
		Topic:       "topic",
		Payload:     "payload-1",
		ErrorMsg:    "first failure",
		RetryCount:  2,
	}
	first.ID = 1
	if err := r.Insert(ctx, first); err != nil {
		t.Fatalf("首次插入死信失败: %v", err)
	}

	second := &model.SysEventInboxDlq{
		ServiceName: "svc",
		EventID:     "event-1",
		Topic:       "topic",
		Payload:     "payload-2",
		ErrorMsg:    "second failure",
		RetryCount:  5,
	}
	second.ID = 2
	if err := r.Insert(ctx, second); err != nil {
		t.Fatalf("重复插入死信不应报错: %v", err)
	}

	var rows []model.SysEventInboxDlq
	if err := gormDB.Find(&rows).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("期望 1 条死信，实际 %d 条", len(rows))
	}
	if rows[0].ID != first.ID || rows[0].ErrorMsg != first.ErrorMsg || rows[0].RetryCount != first.RetryCount {
		t.Fatalf("重复插入应保留首条记录，实际 %+v", rows[0])
	}
}

// TestInboxDlqRepoFetchDue 验证只返回已到重投时间的死信
func TestInboxDlqRepoFetchDue(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInboxDlq{}))
	defer cleanup()

	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	rows := []model.SysEventInboxDlq{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Payload: "payload-1"},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Payload: "payload-2", RetryAfterTime: &past},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic", Payload: "payload-3", RetryAfterTime: &future},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化死信失败: %v", err)
	}

	r := repo.NewInboxDlqRepo(gormDB)
	got, err := r.Fetch(context.Background(), "svc", 10, now)
	if err != nil {
		t.Fatalf("查询到期死信失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 条到期死信，实际 %d 条", len(got))
	}
	for _, row := range got {
		if row.EventID == "event-3" {
			t.Fatalf("未到重投时间的死信不应返回: %+v", row)
		}
	}
}

// TestInboxDlqRepoUpdateRetry 验证更新重投次数与重投时间字段
func TestInboxDlqRepoUpdateRetry(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInboxDlq{}))
	defer cleanup()

	r := repo.NewInboxDlqRepo(gormDB)
	row := model.SysEventInboxDlq{ServiceName: "svc", EventID: "event-1", Topic: "topic"}
	row.ID = 1
	if err := r.Insert(context.Background(), &row); err != nil {
		t.Fatalf("插入死信失败: %v", err)
	}

	lastTime := time.Now().Add(-time.Minute).Truncate(time.Second)
	afterTime := time.Now().Add(10 * time.Minute).Truncate(time.Second)
	if err := r.UpdateRetry(context.Background(), row.ID, 3, lastTime, afterTime); err != nil {
		t.Fatalf("更新重投记录失败: %v", err)
	}

	var stored model.SysEventInboxDlq
	if err := gormDB.First(&stored, row.ID).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if stored.RetryCount != 3 {
		t.Fatalf("重投次数期望 3，实际 %d", stored.RetryCount)
	}
	if stored.RetryLastTime == nil || !stored.RetryLastTime.Equal(lastTime) {
		t.Fatalf("最后重投时间期望 %v，实际 %v", lastTime, stored.RetryLastTime)
	}
	if stored.RetryAfterTime == nil || !stored.RetryAfterTime.Equal(afterTime) {
		t.Fatalf("下次可重投时间期望 %v，实际 %v", afterTime, stored.RetryAfterTime)
	}
}

// TestInboxDlqRepoDelete 验证硬删除死信记录
func TestInboxDlqRepoDelete(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventInboxDlq{}))
	defer cleanup()

	rows := []model.SysEventInboxDlq{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic"},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic"},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化死信失败: %v", err)
	}

	r := repo.NewInboxDlqRepo(gormDB)
	if err := r.Delete(context.Background(), 1); err != nil {
		t.Fatalf("删除死信失败: %v", err)
	}

	var all []model.SysEventInboxDlq
	if err := gormDB.Unscoped().Find(&all).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if len(all) != 1 || all[0].ID != 2 {
		t.Fatalf("期望硬删除后仅剩 event-2，实际 %d 条", len(all))
	}
}
