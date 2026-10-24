-- ----------------------------
-- 消息队列事件模块建表脚本
-- PostgreSQL 版；MySQL 版见 sql/mysql/sys_event.sql，两份字段口径一致。
-- PG 索引名 schema 级全局唯一，故事件表索引/唯一约束名带表前缀（inbox/outbox/outbox_dlq/inbox_dlq）。
-- 作者: zwei
-- 说明:
--   event_id 是全局唯一事件 ID（建议使用雪花 ID），所有 event 表统一遵守此约定；
--   sys_event_outbox.status 对应 EventOutboxStatus: PENDING-待处理, FETCHED-已抓取, SENT_SUCCESS-发送成功, SENT_FAIL-发送失败
-- ----------------------------

CREATE TABLE IF NOT EXISTS sys_event_inbox
(
    id           BIGINT        NOT NULL,
    service_name VARCHAR(100)  NOT NULL,
    event_id     VARCHAR(100)  NOT NULL,
    topic        VARCHAR(200)  NOT NULL,
    payload      TEXT          DEFAULT NULL,
    create_time  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by    BIGINT        DEFAULT NULL,
    update_time  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_by    BIGINT        DEFAULT NULL,
    delete_time  TIMESTAMP     DEFAULT NULL,
    delete_by    BIGINT        DEFAULT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uk_inbox_service_event_id UNIQUE (service_name, event_id)
);

CREATE INDEX IF NOT EXISTS idx_inbox_service_name ON sys_event_inbox (service_name);
CREATE INDEX IF NOT EXISTS idx_inbox_service_create_time ON sys_event_inbox (service_name, create_time);
CREATE INDEX IF NOT EXISTS idx_inbox_create_time ON sys_event_inbox (create_time);

COMMENT ON TABLE sys_event_inbox IS '事件收件箱表';
COMMENT ON COLUMN sys_event_inbox.id IS '主键ID';
COMMENT ON COLUMN sys_event_inbox.service_name IS '归属业务系统名称';
COMMENT ON COLUMN sys_event_inbox.event_id IS '事件ID（全局唯一，用于幂等校验）';
COMMENT ON COLUMN sys_event_inbox.topic IS '事件主题';
COMMENT ON COLUMN sys_event_inbox.payload IS '事件内容负载';
COMMENT ON COLUMN sys_event_inbox.create_time IS '创建时间';
COMMENT ON COLUMN sys_event_inbox.create_by IS '创建人ID';
COMMENT ON COLUMN sys_event_inbox.update_time IS '更新时间';
COMMENT ON COLUMN sys_event_inbox.update_by IS '更新人ID';
COMMENT ON COLUMN sys_event_inbox.delete_time IS '删除时间';
COMMENT ON COLUMN sys_event_inbox.delete_by IS '删除人ID';

CREATE TABLE IF NOT EXISTS sys_event_outbox
(
    id           BIGINT        NOT NULL,
    service_name VARCHAR(100)  NOT NULL,
    event_id     VARCHAR(100)  NOT NULL,
    topic        VARCHAR(200)  NOT NULL,
    payload      TEXT          DEFAULT NULL,
    status       VARCHAR(20)   NOT NULL DEFAULT 'PENDING',
    fetch_time   TIMESTAMP     DEFAULT NULL,
    sent_time    TIMESTAMP     DEFAULT NULL,
    create_time  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by    BIGINT        DEFAULT NULL,
    update_time  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_by    BIGINT        DEFAULT NULL,
    delete_time  TIMESTAMP     DEFAULT NULL,
    delete_by    BIGINT        DEFAULT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uk_outbox_service_event_id UNIQUE (service_name, event_id)
);

CREATE INDEX IF NOT EXISTS idx_outbox_service_status_create_time ON sys_event_outbox (service_name, status, create_time);
CREATE INDEX IF NOT EXISTS idx_outbox_service_status_fetch_time ON sys_event_outbox (service_name, status, fetch_time);
CREATE INDEX IF NOT EXISTS idx_outbox_service_status_sent_time ON sys_event_outbox (service_name, status, sent_time);
CREATE INDEX IF NOT EXISTS idx_outbox_event_id ON sys_event_outbox (event_id);

COMMENT ON TABLE sys_event_outbox IS '事件发件箱表';
COMMENT ON COLUMN sys_event_outbox.id IS '主键ID';
COMMENT ON COLUMN sys_event_outbox.service_name IS '归属业务系统名称';
COMMENT ON COLUMN sys_event_outbox.event_id IS '事件ID（全局唯一）';
COMMENT ON COLUMN sys_event_outbox.topic IS '事件主题';
COMMENT ON COLUMN sys_event_outbox.payload IS '事件内容负载';
COMMENT ON COLUMN sys_event_outbox.status IS '事件状态: PENDING-待处理, FETCHED-已抓取, SENT_SUCCESS-发送成功, SENT_FAIL-发送失败';
COMMENT ON COLUMN sys_event_outbox.fetch_time IS '事件抓取时间';
COMMENT ON COLUMN sys_event_outbox.sent_time IS '事件发送成功时间';
COMMENT ON COLUMN sys_event_outbox.create_time IS '创建时间';
COMMENT ON COLUMN sys_event_outbox.create_by IS '创建人ID';
COMMENT ON COLUMN sys_event_outbox.update_time IS '更新时间';
COMMENT ON COLUMN sys_event_outbox.update_by IS '更新人ID';
COMMENT ON COLUMN sys_event_outbox.delete_time IS '删除时间';
COMMENT ON COLUMN sys_event_outbox.delete_by IS '删除人ID';

CREATE TABLE IF NOT EXISTS sys_event_outbox_dlq
(
    id                BIGINT        NOT NULL,
    service_name      VARCHAR(100)  NOT NULL,
    event_outbox_id   BIGINT        NOT NULL,
    error_msg         TEXT          DEFAULT NULL,
    retry_count       INT           NOT NULL DEFAULT 0,
    retry_last_time   TIMESTAMP     DEFAULT NULL,
    retry_after_time  TIMESTAMP     DEFAULT NULL,
    create_time       TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by         BIGINT        DEFAULT NULL,
    update_time       TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_by         BIGINT        DEFAULT NULL,
    delete_time       TIMESTAMP     DEFAULT NULL,
    delete_by         BIGINT        DEFAULT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uk_outbox_dlq_event_outbox_id UNIQUE (event_outbox_id)
);

CREATE INDEX IF NOT EXISTS idx_outbox_dlq_service_name_create_time ON sys_event_outbox_dlq (service_name, create_time);
CREATE INDEX IF NOT EXISTS idx_outbox_dlq_retry_last_time ON sys_event_outbox_dlq (retry_last_time);
CREATE INDEX IF NOT EXISTS idx_outbox_dlq_retry_after_time ON sys_event_outbox_dlq (retry_after_time);

COMMENT ON TABLE sys_event_outbox_dlq IS '事件发件箱死信队列表';
COMMENT ON COLUMN sys_event_outbox_dlq.id IS '主键ID';
COMMENT ON COLUMN sys_event_outbox_dlq.service_name IS '归属业务系统名称';
COMMENT ON COLUMN sys_event_outbox_dlq.event_outbox_id IS '发件箱ID';
COMMENT ON COLUMN sys_event_outbox_dlq.error_msg IS '失败信息';
COMMENT ON COLUMN sys_event_outbox_dlq.retry_count IS '重试次数';
COMMENT ON COLUMN sys_event_outbox_dlq.retry_last_time IS '最后重试时间';
COMMENT ON COLUMN sys_event_outbox_dlq.retry_after_time IS '最早可重试时间';
COMMENT ON COLUMN sys_event_outbox_dlq.create_time IS '创建时间';
COMMENT ON COLUMN sys_event_outbox_dlq.create_by IS '创建人ID';
COMMENT ON COLUMN sys_event_outbox_dlq.update_time IS '更新时间';
COMMENT ON COLUMN sys_event_outbox_dlq.update_by IS '更新人ID';
COMMENT ON COLUMN sys_event_outbox_dlq.delete_time IS '删除时间';
COMMENT ON COLUMN sys_event_outbox_dlq.delete_by IS '删除人ID';

CREATE TABLE IF NOT EXISTS sys_event_inbox_dlq
(
    id                BIGINT        NOT NULL,
    service_name      VARCHAR(100)  NOT NULL,
    event_id          VARCHAR(100)  NOT NULL,
    topic             VARCHAR(200)  NOT NULL,
    payload           TEXT          DEFAULT NULL,
    error_msg         TEXT          DEFAULT NULL,
    retry_count       INT           NOT NULL DEFAULT 0,
    retry_last_time   TIMESTAMP     DEFAULT NULL,
    retry_after_time  TIMESTAMP     DEFAULT NULL,
    create_time       TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by         BIGINT        DEFAULT NULL,
    update_time       TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_by         BIGINT        DEFAULT NULL,
    delete_time       TIMESTAMP     DEFAULT NULL,
    delete_by         BIGINT        DEFAULT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uk_inbox_dlq_service_event_id UNIQUE (service_name, event_id)
);

CREATE INDEX IF NOT EXISTS idx_inbox_dlq_service_create_time ON sys_event_inbox_dlq (service_name, create_time);
CREATE INDEX IF NOT EXISTS idx_inbox_dlq_service_retry_after_time ON sys_event_inbox_dlq (service_name, retry_after_time);

COMMENT ON TABLE sys_event_inbox_dlq IS '事件收件箱死信表';
COMMENT ON COLUMN sys_event_inbox_dlq.id IS '主键ID';
COMMENT ON COLUMN sys_event_inbox_dlq.service_name IS '归属业务系统名称';
COMMENT ON COLUMN sys_event_inbox_dlq.event_id IS '事件ID（全局唯一）';
COMMENT ON COLUMN sys_event_inbox_dlq.topic IS '事件主题';
COMMENT ON COLUMN sys_event_inbox_dlq.payload IS '事件内容负载';
COMMENT ON COLUMN sys_event_inbox_dlq.error_msg IS '失败信息';
COMMENT ON COLUMN sys_event_inbox_dlq.retry_count IS '已重试次数（不含首次处理）';
COMMENT ON COLUMN sys_event_inbox_dlq.retry_last_time IS '最后重投时间';
COMMENT ON COLUMN sys_event_inbox_dlq.retry_after_time IS '最早可重投时间';
COMMENT ON COLUMN sys_event_inbox_dlq.create_time IS '创建时间';
COMMENT ON COLUMN sys_event_inbox_dlq.create_by IS '创建人ID';
COMMENT ON COLUMN sys_event_inbox_dlq.update_time IS '更新时间';
COMMENT ON COLUMN sys_event_inbox_dlq.update_by IS '更新人ID';
COMMENT ON COLUMN sys_event_inbox_dlq.delete_time IS '删除时间';
COMMENT ON COLUMN sys_event_inbox_dlq.delete_by IS '删除人ID';
