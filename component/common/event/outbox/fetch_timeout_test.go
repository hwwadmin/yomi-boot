// Package outbox 的发件箱超时回收器测试
//
// @author zwei
package outbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
)

// ptrTime 返回时间指针，便于构造测试数据
func ptrTime(t time.Time) *time.Time { return &t }

// TestFetchTimeoutDoTask 验证只重置停留 FETCHED 超时的记录
func TestFetchTimeoutDoTask(t *testing.T) {
	gormDB, outboxRepo, _, cleanup := newOutboxTestDB(t)
	defer cleanup()

	oldTime := time.Now().Add(-10 * time.Minute)
	freshTime := time.Now()
	rows := []model.SysEventOutbox{
		{ServiceName: "svc", EventID: "event-1", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &oldTime},
		{ServiceName: "svc", EventID: "event-2", Topic: "topic", Status: model.OutboxStatusFetched, FetchTime: &freshTime},
		{ServiceName: "svc", EventID: "event-3", Topic: "topic", Status: model.OutboxStatusPending, FetchTime: &oldTime},
		{ServiceName: "svc", EventID: "event-4", Topic: "topic", Status: model.OutboxStatusSentSuccess, FetchTime: &oldTime},
	}
	for i := range rows {
		rows[i].ID = int64(i + 1)
	}
	if err := gormDB.Create(&rows).Error; err != nil {
		t.Fatalf("初始化发件箱数据失败: %v", err)
	}

	f := NewFetchTimeout(cfg.Runtime{ServiceName: "svc", FetchTimeout: 5 * time.Minute}, outboxRepo)
	if err := f.doTask(context.Background()); err != nil {
		t.Fatalf("超时回收任务失败: %v", err)
	}

	var stored []model.SysEventOutbox
	if err := gormDB.Order("id ASC").Find(&stored).Error; err != nil {
		t.Fatalf("查询发件箱失败: %v", err)
	}
	if stored[0].Status != model.OutboxStatusPending || stored[0].FetchTime != nil {
		t.Fatalf("超时 FETCHED 记录应重置为 PENDING: %+v", stored[0])
	}
	if stored[1].Status != model.OutboxStatusFetched || stored[1].FetchTime == nil {
		t.Fatalf("未超时 FETCHED 记录不应被重置: %+v", stored[1])
	}
	if stored[2].Status != model.OutboxStatusPending {
		t.Fatalf("PENDING 记录不应受影响: %+v", stored[2])
	}
	if stored[3].Status != model.OutboxStatusSentSuccess {
		t.Fatalf("已成功记录不应被重置: %+v", stored[3])
	}
}

// TestFetchTimeoutDoTaskEmpty 验证无超时记录时直接返回
func TestFetchTimeoutDoTaskEmpty(t *testing.T) {
	_, outboxRepo, _, cleanup := newOutboxTestDB(t)
	defer cleanup()

	f := NewFetchTimeout(cfg.Runtime{ServiceName: "svc", FetchTimeout: 5 * time.Minute}, outboxRepo)
	if err := f.doTask(context.Background()); err != nil {
		t.Fatalf("无超时记录时不应报错: %v", err)
	}
}

// TestFetchTimeoutStartRejectsNilFactory 验证缺少分布式锁工厂时启动报错
func TestFetchTimeoutStartRejectsNilFactory(t *testing.T) {
	_, outboxRepo, _, cleanup := newOutboxTestDB(t)
	defer cleanup()
	f := NewFetchTimeout(cfg.Runtime{ServiceName: "svc", FetchFrequency: 50 * time.Millisecond}, outboxRepo)
	if err := f.Start(context.Background(), nil); err == nil {
		t.Fatal("nil 分布式锁工厂应返回错误")
	}
}

// TestBuildOutboxSample 验证日志采样拼接与数量上限
func TestBuildOutboxSample(t *testing.T) {
	if got := buildOutboxSample(nil); got != "" {
		t.Fatalf("空记录期望空串，实际 %q", got)
	}

	rows := []model.SysEventOutbox{
		{EventID: "e1", Topic: "t1"},
		{EventID: "e2", Topic: "t2", FetchTime: ptrTime(time.Now())},
	}
	got := buildOutboxSample(rows)
	if !strings.Contains(got, "eventId=e1|topic=t1|fetchTime=") {
		t.Fatalf("无抓取时间的记录应输出空 fetchTime: %q", got)
	}
	if !strings.Contains(got, "eventId=e2|topic=t2|fetchTime=") {
		t.Fatalf("采样应包含有抓取时间的记录: %q", got)
	}
	if strings.Count(got, "eventId=") != 2 {
		t.Fatalf("采样条数期望 2，实际 %q", got)
	}

	many := make([]model.SysEventOutbox, 0, fetchTimeoutLogSampleSize+5)
	for i := 0; i < fetchTimeoutLogSampleSize+5; i++ {
		many = append(many, model.SysEventOutbox{EventID: "e", Topic: "t"})
	}
	if got := buildOutboxSample(many); strings.Count(got, "eventId=") != fetchTimeoutLogSampleSize {
		t.Fatalf("采样条数应不超过 %d，实际 %q", fetchTimeoutLogSampleSize, got)
	}
}
