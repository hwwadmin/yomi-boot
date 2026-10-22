// Package event 提供发件箱事件发布能力
//
// @author zwei
package event

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake"
	"gitee.com/hq_project/yomi-boot/core/errors"
	"gitee.com/hq_project/yomi-boot/core/shared/enums"
)

// Publisher 发件箱事件发布器
type Publisher struct {
	repo   *repo.OutboxRepo
	cfg    cfg.Runtime
	idGen  snowflake.Generator
	dbType enums.DbType
}

// NewPublisher 创建发件箱事件发布器
func NewPublisher(
	outboxRepo *repo.OutboxRepo,
	runtime cfg.Runtime,
	idGen snowflake.Generator,
	dbType enums.DbType,
) *Publisher {
	return &Publisher{
		repo:   outboxRepo,
		cfg:    runtime,
		idGen:  idGen,
		dbType: dbType,
	}
}

// Publish 将事件写入发件箱，应在业务事务内调用以保证与业务数据原子提交。
// evt.EventID 必须全局唯一（建议使用雪花 ID）；同一 service 下重复发布会返回 ErrDuplicatePublish。
func (p *Publisher) Publish(ctx context.Context, evt Event) error {
	if evt.EventID == "" || evt.Topic == "" {
		return errors.Sentinel(ErrEventInvalid)
	}

	id, err := p.idGen.NextID()
	if err != nil {
		return err
	}
	row := &model.SysEventOutbox{
		ServiceName: p.cfg.ServiceName,
		EventID:     evt.EventID,
		Topic:       evt.Topic,
		Payload:     evt.Payload,
		Status:      model.OutboxStatusPending,
		BaseModel: db.BaseModel{
			ID: id,
		},
	}
	if err := p.repo.Insert(ctx, row); err != nil {
		if db.IsDuplicateKey(err, p.dbType) {
			return errors.Wrap(ErrDuplicatePublish, "serviceName=%s eventId=%s", p.cfg.ServiceName, evt.EventID)
		}
		return err
	}

	logger.Info(ctx, "消息队列事件发布持久化成功", "eventId", evt.EventID, "topic", evt.Topic)
	return nil
}
