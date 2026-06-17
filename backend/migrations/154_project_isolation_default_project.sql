-- Project isolation foundation.
-- Creates the default project and moves all existing resources into it.
-- Legacy operator permissions are intentionally not migrated to project membership.

CREATE TABLE IF NOT EXISTS projects (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(80) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    profiles JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS projects_slug_key
    ON projects(slug);
CREATE INDEX IF NOT EXISTS idx_projects_status
    ON projects(status);
CREATE INDEX IF NOT EXISTS idx_projects_deleted_at
    ON projects(deleted_at);

INSERT INTO projects (name, slug, description, status, profiles, created_at, updated_at)
VALUES ('默认项目', 'default', 'Migrated default project for existing resources.', 'active', '{}'::jsonb, NOW(), NOW())
ON CONFLICT (slug) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

CREATE TABLE IF NOT EXISTS project_members (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_owner BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_project_members_user_id
    ON project_members(user_id);
CREATE INDEX IF NOT EXISTS idx_project_members_project_role
    ON project_members(project_id, role);

CREATE TEMP TABLE legacy_user_roles ON COMMIT DROP AS
SELECT id, role
FROM users
WHERE role IN ('admin', 'operator')
  AND deleted_at IS NULL;

-- Global role migration:
-- - Existing admins become super admins.
-- - Existing operators become ordinary users; old operator_group_scopes is kept only as legacy audit data.
UPDATE users
SET role = 'super_admin', updated_at = NOW()
WHERE id IN (SELECT id FROM legacy_user_roles WHERE role = 'admin');

UPDATE users
SET role = 'user', updated_at = NOW()
WHERE id IN (SELECT id FROM legacy_user_roles WHERE role = 'operator');

INSERT INTO project_members (project_id, user_id, role, scopes, is_owner, created_at, updated_at)
SELECT
    p.id,
    u.id,
    CASE
        WHEN lur.role = 'admin' OR u.role = 'super_admin' THEN 'admin'
        ELSE 'user'
    END,
    '[]'::jsonb,
    lur.role = 'admin' OR u.role = 'super_admin',
    NOW(),
    NOW()
FROM projects p
JOIN users u ON u.deleted_at IS NULL
LEFT JOIN legacy_user_roles lur ON lur.id = u.id
WHERE p.slug = 'default'
ON CONFLICT (project_id, user_id) DO NOTHING;

DROP TABLE legacy_user_roles;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS project_id BIGINT;
ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS project_id BIGINT;
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS project_id BIGINT;
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS project_id BIGINT;

UPDATE groups
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

UPDATE accounts
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

UPDATE api_keys
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

UPDATE usage_logs
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

ALTER TABLE groups
    ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE accounts
    ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE api_keys
    ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE usage_logs
    ALTER COLUMN project_id SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_project_id_fkey') THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'accounts_project_id_fkey') THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'api_keys_project_id_fkey') THEN
        ALTER TABLE api_keys
            ADD CONSTRAINT api_keys_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'usage_logs_project_id_fkey') THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_groups_project_id
    ON groups(project_id);
CREATE INDEX IF NOT EXISTS idx_groups_project_platform_status
    ON groups(project_id, platform, status);
CREATE INDEX IF NOT EXISTS idx_accounts_project_id
    ON accounts(project_id);
CREATE INDEX IF NOT EXISTS idx_accounts_project_platform_priority
    ON accounts(project_id, platform, priority);
CREATE INDEX IF NOT EXISTS idx_api_keys_project_id
    ON api_keys(project_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_project_status
    ON api_keys(project_id, status);
CREATE INDEX IF NOT EXISTS idx_usage_logs_project_id
    ON usage_logs(project_id);
CREATE INDEX IF NOT EXISTS idx_usage_logs_project_created_at
    ON usage_logs(project_id, created_at);

ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS project_id BIGINT;
ALTER TABLE ops_system_metrics
    ADD COLUMN IF NOT EXISTS project_id BIGINT;
ALTER TABLE ops_metrics_hourly
    ADD COLUMN IF NOT EXISTS project_id BIGINT;
ALTER TABLE ops_metrics_daily
    ADD COLUMN IF NOT EXISTS project_id BIGINT;
ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS project_id BIGINT;
ALTER TABLE ops_system_logs
    ADD COLUMN IF NOT EXISTS project_id BIGINT;

UPDATE ops_error_logs e
SET project_id = COALESCE(
    (SELECT ak.project_id FROM api_keys ak WHERE ak.id = e.api_key_id),
    (SELECT a.project_id FROM accounts a WHERE a.id = e.account_id),
    (SELECT id FROM projects WHERE slug = 'default')
)
WHERE e.project_id IS NULL
  AND (e.api_key_id IS NOT NULL OR e.account_id IS NOT NULL);

UPDATE ops_error_logs
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

UPDATE ops_system_metrics
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

UPDATE ops_metrics_hourly
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

UPDATE ops_metrics_daily
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

UPDATE ops_alert_events
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

UPDATE ops_system_logs
SET project_id = (SELECT id FROM projects WHERE slug = 'default')
WHERE project_id IS NULL;

ALTER TABLE ops_error_logs
    ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE ops_system_metrics
    ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE ops_metrics_hourly
    ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE ops_metrics_daily
    ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE ops_alert_events
    ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE ops_system_logs
    ALTER COLUMN project_id SET NOT NULL;

DROP INDEX IF EXISTS idx_ops_metrics_hourly_unique_dim;
CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_metrics_hourly_unique_dim
    ON ops_metrics_hourly (
        project_id,
        bucket_start,
        COALESCE(platform, ''),
        COALESCE(group_id, 0)
    );

DROP INDEX IF EXISTS idx_ops_metrics_daily_unique_dim;
CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_metrics_daily_unique_dim
    ON ops_metrics_daily (
        project_id,
        bucket_date,
        COALESCE(platform, ''),
        COALESCE(group_id, 0)
    );

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ops_error_logs_project_id_fkey') THEN
        ALTER TABLE ops_error_logs
            ADD CONSTRAINT ops_error_logs_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ops_system_metrics_project_id_fkey') THEN
        ALTER TABLE ops_system_metrics
            ADD CONSTRAINT ops_system_metrics_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ops_metrics_hourly_project_id_fkey') THEN
        ALTER TABLE ops_metrics_hourly
            ADD CONSTRAINT ops_metrics_hourly_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ops_metrics_daily_project_id_fkey') THEN
        ALTER TABLE ops_metrics_daily
            ADD CONSTRAINT ops_metrics_daily_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ops_alert_events_project_id_fkey') THEN
        ALTER TABLE ops_alert_events
            ADD CONSTRAINT ops_alert_events_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ops_system_logs_project_id_fkey') THEN
        ALTER TABLE ops_system_logs
            ADD CONSTRAINT ops_system_logs_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_project_time
    ON ops_error_logs(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ops_system_metrics_project_time
    ON ops_system_metrics(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ops_metrics_hourly_project_time
    ON ops_metrics_hourly(project_id, bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_ops_metrics_daily_project_time
    ON ops_metrics_daily(project_id, bucket_date DESC);
CREATE INDEX IF NOT EXISTS idx_ops_alert_events_project_time
    ON ops_alert_events(project_id, fired_at DESC);
CREATE INDEX IF NOT EXISTS idx_ops_system_logs_project_time
    ON ops_system_logs(project_id, created_at DESC);
