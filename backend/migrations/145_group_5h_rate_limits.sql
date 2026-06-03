-- 分组级 5 小时 USD 用量限制。
-- groups.rate_limit_5h 是配置；user_group_rate_limit_windows 存储每个 (user_id, group_id) 的窗口用量。
-- 0 = 不限制，正数 = 该用户在该分组 5 小时窗口内的 USD 上限。

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS rate_limit_5h DECIMAL(20,8) NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS user_group_rate_limit_windows (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id          BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    usage_5h_usd      DECIMAL(20,10) NOT NULL DEFAULT 0,
    window_5h_start   TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS usergroupratelimitwindow_user_id_group_id_uq
    ON user_group_rate_limit_windows (user_id, group_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS usergroupratelimitwindow_user_id
    ON user_group_rate_limit_windows (user_id);

CREATE INDEX IF NOT EXISTS usergroupratelimitwindow_group_id
    ON user_group_rate_limit_windows (group_id);
