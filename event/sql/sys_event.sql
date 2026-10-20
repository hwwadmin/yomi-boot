-- ----------------------------
-- 消息队列事件模块建表脚本
-- 作者: zwei
-- 说明:
--   event_id 是全局唯一事件 ID（建议使用雪花 ID），所有 event 表统一遵守此约定；
--   sys_event_outbox.status 对应 EventOutboxStatus: PENDING-待处理, FETCHED-已抓取, SENT_SUCCESS-发送成功, SENT_FAIL-发送失败
-- ----------------------------

CREATE TABLE IF NOT EXISTS `sys_event_inbox`
(
    `id`           bigint       NOT NULL COMMENT '主键ID',
    `service_name` varchar(100) NOT NULL COMMENT '归属业务系统名称',
    `event_id`     varchar(100) NOT NULL COMMENT '事件ID（全局唯一，用于幂等校验）',
    `topic`        varchar(200) NOT NULL COMMENT '事件主题',
    `payload`      longtext              DEFAULT NULL COMMENT '事件内容负载',
    `create_time`  datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `create_by`    bigint                DEFAULT NULL COMMENT '创建人ID',
    `update_time`  datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `update_by`    bigint                DEFAULT NULL COMMENT '更新人ID',
    `delete_time`  datetime              DEFAULT NULL COMMENT '删除时间',
    `delete_by`    bigint                DEFAULT NULL COMMENT '删除人ID',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_service_event_id` (`service_name`, `event_id`),
    KEY `idx_service_name` (`service_name`),
    KEY `idx_service_create_time` (`service_name`, `create_time`),
    KEY `idx_create_time` (`create_time`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_unicode_ci
  COMMENT = '事件收件箱表';

CREATE TABLE IF NOT EXISTS `sys_event_outbox`
(
    `id`           bigint       NOT NULL COMMENT '主键ID',
    `service_name` varchar(100) NOT NULL COMMENT '归属业务系统名称',
    `event_id`     varchar(100) NOT NULL COMMENT '事件ID（全局唯一）',
    `topic`        varchar(200) NOT NULL COMMENT '事件主题',
    `payload`      longtext              DEFAULT NULL COMMENT '事件内容负载',
    `status`       varchar(20)  NOT NULL DEFAULT 'PENDING' COMMENT '事件状态: PENDING-待处理, FETCHED-已抓取, SENT_SUCCESS-发送成功, SENT_FAIL-发送失败',
    `fetch_time`   datetime              DEFAULT NULL COMMENT '事件抓取时间',
    `sent_time`    datetime              DEFAULT NULL COMMENT '事件发送成功时间',
    `create_time`  datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `create_by`    bigint                DEFAULT NULL COMMENT '创建人ID',
    `update_time`  datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `update_by`    bigint                DEFAULT NULL COMMENT '更新人ID',
    `delete_time`  datetime              DEFAULT NULL COMMENT '删除时间',
    `delete_by`    bigint                DEFAULT NULL COMMENT '删除人ID',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_service_event_id` (`service_name`, `event_id`),
    KEY `idx_service_status_create_time` (`service_name`, `status`, `create_time`),
    KEY `idx_service_status_fetch_time` (`service_name`, `status`, `fetch_time`),
    KEY `idx_service_status_sent_time` (`service_name`, `status`, `sent_time`),
    KEY `idx_event_id` (`event_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_unicode_ci
  COMMENT = '事件发件箱表';

CREATE TABLE IF NOT EXISTS `sys_event_outbox_dlq`
(
    `id`              bigint       NOT NULL COMMENT '主键ID',
    `service_name`    varchar(100) NOT NULL COMMENT '归属业务系统名称',
    `event_outbox_id` bigint       NOT NULL COMMENT '发件箱ID',
    `error_msg`       text                  DEFAULT NULL COMMENT '失败信息',
    `retry_count`     int          NOT NULL DEFAULT 0 COMMENT '重试次数',
    `retry_last_time` datetime              DEFAULT NULL COMMENT '最后重试时间',
    `retry_after_time` datetime             DEFAULT NULL COMMENT '最早可重试时间',
    `create_time`     datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `create_by`       bigint                DEFAULT NULL COMMENT '创建人ID',
    `update_time`     datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `update_by`       bigint                DEFAULT NULL COMMENT '更新人ID',
    `delete_time`     datetime              DEFAULT NULL COMMENT '删除时间',
    `delete_by`       bigint                DEFAULT NULL COMMENT '删除人ID',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_event_outbox_id` (`event_outbox_id`),
    KEY `idx_service_name_create_time` (`service_name`, `create_time`),
    KEY `idx_retry_last_time` (`retry_last_time`),
    KEY `idx_retry_after_time` (`retry_after_time`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_unicode_ci
  COMMENT = '事件发件箱死信队列表';

CREATE TABLE IF NOT EXISTS `sys_event_inbox_dlq`
(
    `id`           bigint       NOT NULL COMMENT '主键ID',
    `service_name` varchar(100) NOT NULL COMMENT '归属业务系统名称',
    `event_id`     varchar(100) NOT NULL COMMENT '事件ID（全局唯一）',
    `topic`        varchar(200) NOT NULL COMMENT '事件主题',
    `payload`      longtext              DEFAULT NULL COMMENT '事件内容负载',
    `error_msg`    text                  DEFAULT NULL COMMENT '失败信息',
    `retry_count`  int          NOT NULL DEFAULT 0 COMMENT '已重试次数（不含首次处理）',
    `retry_last_time` datetime           DEFAULT NULL COMMENT '最后重投时间',
    `retry_after_time` datetime          DEFAULT NULL COMMENT '最早可重投时间',
    `create_time`  datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `create_by`    bigint                DEFAULT NULL COMMENT '创建人ID',
    `update_time`  datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `update_by`    bigint                DEFAULT NULL COMMENT '更新人ID',
    `delete_time`  datetime              DEFAULT NULL COMMENT '删除时间',
    `delete_by`    bigint                DEFAULT NULL COMMENT '删除人ID',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_service_event_id` (`service_name`, `event_id`),
    KEY `idx_service_create_time` (`service_name`, `create_time`),
    KEY `idx_service_retry_after_time` (`service_name`, `retry_after_time`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_unicode_ci
  COMMENT = '事件收件箱死信表';
