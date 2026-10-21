-- 平台共享 DDL：业务幂等表（各领域库内各建一份）
-- 权威定义仅此一份；与 boot/idempotency 封装口径一致。

CREATE TABLE IF NOT EXISTS `sys_biz` (
    `id` bigint NOT NULL COMMENT '主键ID',
    `biz_id` varchar(100) NOT NULL COMMENT '业务全局ID（一次 Saga/业务意图）',
    `biz_sub_id` varchar(100) NOT NULL COMMENT '业务子ID（单次领域写调用）',
    `operation_type` varchar(32) NOT NULL COMMENT '操作类型（如 UPDATE_USER_STATUS）',
    `status` tinyint NOT NULL DEFAULT 1 COMMENT '1-处理中 2-已完成',
    `response_data` json DEFAULT NULL COMMENT '成功时的返回结果缓存（用于重试直接返回）',
    `create_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `create_by` bigint DEFAULT NULL COMMENT '创建人ID',
    `update_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `update_by` bigint DEFAULT NULL COMMENT '更新人ID',
    `delete_time` datetime DEFAULT NULL COMMENT '删除时间',
    `delete_by` bigint DEFAULT NULL COMMENT '删除人ID',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_biz` (`biz_id`, `biz_sub_id`),
    KEY `idx_operation` (`operation_type`),
    KEY `idx_create_time` (`create_time`),
    KEY `idx_status_update_time` (`status`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='业务幂等记录表';
