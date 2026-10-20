// Package repo 的收件箱仓储测试
//
// @author zwei
package repo_test

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/db/dbtest"
	"gitee.com/hq_project/yomi-boot/event/model"
	"gitee.com/hq_project/yomi-boot/event/repo"
)

// TestInboxRepoListConsumedBefore 验证只返回已过保留期且无死信引用的收件箱记录
func TestInboxRepoListConsumedBefore(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(model.SysEventInbox{}, model.SysEventInboxDlq{}),
	)
	defer cleanup()

	oldTime := time.Now().Add(-40 * 24 * time.Hour)
	recentTime := time.Now().Add(-24 * time.Hour)
	rows := []model.SysEventInbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic"},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic"},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic"},
	}
	rows[0].CreateTime = oldTime
	rows[1].CreateTime = recentTime
	rows[2].CreateTime = oldTime
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化收件箱数据失败: %v", err)
	}
	dlq := &model.SysEventInboxDlq{ServiceName: "svc", EventID: "event-3", Topic: "topic", ErrorMsg: "failed"}
	dlq.ID = 100
	if err := gormDB.Create(dlq).Error; err != nil {
		t.Fatalf("初始化死信数据失败: %v", err)
	}

	r := repo.NewInboxRepo(gormDB)
	ids, err := r.ListConsumedBefore(context.Background(), "svc", time.Now().Add(-30*24*time.Hour), 100)
	if err != nil {
		t.Fatalf("查询可清理收件箱记录失败: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("期望只返回 event-1，实际 %v", ids)
	}
	if got, err := r.ListConsumedBefore(context.Background(), "svc", time.Now(), 0); err != nil || got != nil {
		t.Fatalf("limit<=0 应返回空，实际 ids=%v err=%v", got, err)
	}
}
