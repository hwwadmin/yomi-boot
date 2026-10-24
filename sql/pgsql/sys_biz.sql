-- 平台共享 DDL：业务幂等表（各领域库内各建一份）
-- PostgreSQL 版；MySQL 版见 sql/mysql/sys_biz.sql，两份字段口径一致。
-- 与 component/idempotency 封装口径一致。
-- update_time 的 MySQL ON UPDATE 语义由应用层（GORM autoUpdateTime）维护，PG 侧不设触发器。

CREATE TABLE IF NOT EXISTS sys_biz (
    id             BIGINT       NOT NULL,
    biz_id         VARCHAR(100) NOT NULL,
    biz_sub_id     VARCHAR(100) NOT NULL,
    operation_type VARCHAR(32)  NOT NULL,
    status         SMALLINT     NOT NULL DEFAULT 1,
    response_data  JSONB        DEFAULT NULL,
    create_time    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by      BIGINT       DEFAULT NULL,
    update_time    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_by      BIGINT       DEFAULT NULL,
    delete_time    TIMESTAMP    DEFAULT NULL,
    delete_by      BIGINT       DEFAULT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uk_biz UNIQUE (biz_id, biz_sub_id)
);

CREATE INDEX IF NOT EXISTS idx_operation ON sys_biz (operation_type);
CREATE INDEX IF NOT EXISTS idx_create_time ON sys_biz (create_time);
CREATE INDEX IF NOT EXISTS idx_status_update_time ON sys_biz (status, update_time);

COMMENT ON TABLE sys_biz IS '业务幂等记录表';
COMMENT ON COLUMN sys_biz.id IS '主键ID';
COMMENT ON COLUMN sys_biz.biz_id IS '业务全局ID（一次 Saga/业务意图）';
COMMENT ON COLUMN sys_biz.biz_sub_id IS '业务子ID（单次领域写调用）';
COMMENT ON COLUMN sys_biz.operation_type IS '操作类型（如 UPDATE_USER_STATUS）';
COMMENT ON COLUMN sys_biz.status IS '1-处理中 2-已完成';
COMMENT ON COLUMN sys_biz.response_data IS '成功时的返回结果缓存（用于重试直接返回）';
COMMENT ON COLUMN sys_biz.create_time IS '创建时间';
COMMENT ON COLUMN sys_biz.create_by IS '创建人ID';
COMMENT ON COLUMN sys_biz.update_time IS '更新时间';
COMMENT ON COLUMN sys_biz.update_by IS '更新人ID';
COMMENT ON COLUMN sys_biz.delete_time IS '删除时间';
COMMENT ON COLUMN sys_biz.delete_by IS '删除人ID';
