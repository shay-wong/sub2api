-- Operator admin-console account scope.
-- This is intentionally separate from user_allowed_groups: the latter controls
-- normal API-key group usage, while this table controls which accounts an
-- operator may see and manage in the admin console.
CREATE TABLE IF NOT EXISTS operator_group_scopes (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_operator_group_scopes_group_id
    ON operator_group_scopes(group_id);
