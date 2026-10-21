-- 平台共享 DDL：DTM 子事务屏障表（各领域库内各建一份，与业务表同库）
-- 权威定义仅此一份；与 cloud/dtm 封装口径一致，勿在各服务 script/sql 下复制。

CREATE TABLE IF NOT EXISTS `sys_barrier` (
    `id`         BIGINT       NOT NULL AUTO_INCREMENT,
    `trans_type` VARCHAR(45)  NOT NULL DEFAULT '',
    `gid`        VARCHAR(128) NOT NULL DEFAULT '',
    `branch_id`  VARCHAR(128) NOT NULL DEFAULT '',
    `op`         VARCHAR(45)  NOT NULL DEFAULT '',
    `barrier_id` VARCHAR(45)  NOT NULL DEFAULT '',
    `reason`     VARCHAR(45)  NOT NULL DEFAULT '' COMMENT 'rollback/timeout 等',
    `create_time` DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `update_time` DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_barrier` (`trans_type`, `gid`, `branch_id`, `op`, `barrier_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='DTM 子事务屏障';
