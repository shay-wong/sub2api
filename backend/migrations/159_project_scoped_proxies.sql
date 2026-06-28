-- Give proxies the same project origin/default column as accounts and groups.
-- Visibility and management follow active project profile bindings, so one proxy can remain shared by projects.

ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS project_id BIGINT;

UPDATE proxies
SET project_id = dp.id
FROM (
    SELECT id
    FROM projects
    WHERE slug = 'default'
      AND deleted_at IS NULL
    ORDER BY id
    LIMIT 1
) AS dp
WHERE proxies.project_id IS NULL;

ALTER TABLE proxies
    ALTER COLUMN project_id SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'proxies_projects_proxies'
    ) THEN
        ALTER TABLE proxies
            ADD CONSTRAINT proxies_projects_proxies
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_proxies_project_id
    ON proxies(project_id);

CREATE INDEX IF NOT EXISTS idx_proxies_project_status
    ON proxies(project_id, status);

ALTER TABLE project_profile_bindings
    DROP CONSTRAINT IF EXISTS project_profile_bindings_resource_type_check;

ALTER TABLE project_profile_bindings
    ADD CONSTRAINT project_profile_bindings_resource_type_check
        CHECK (resource_type IN ('user', 'group', 'account', 'proxy', 'subscription', 'api_key'));

INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
SELECT DISTINCT pp.id, 'proxy', p.id, NOW(), '{}'::jsonb
FROM project_profiles pp
JOIN proxies p ON p.project_id = pp.project_id
WHERE pp.is_active = TRUE
  AND pp.deleted_at IS NULL
  AND p.deleted_at IS NULL
ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING;

INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
SELECT DISTINCT pp.id, 'proxy', p.id, NOW(), '{}'::jsonb
FROM proxies p
JOIN accounts a ON a.proxy_id = p.id
JOIN project_profiles pp ON pp.project_id = a.project_id
WHERE pp.is_active = TRUE
  AND pp.deleted_at IS NULL
  AND p.deleted_at IS NULL
  AND a.deleted_at IS NULL
ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING;

INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
SELECT DISTINCT pp.id, 'proxy', bp.id, NOW(), '{}'::jsonb
FROM proxies p
JOIN proxies bp ON bp.id = p.backup_proxy_id
JOIN accounts a ON a.proxy_id = p.id
JOIN project_profiles pp ON pp.project_id = a.project_id
WHERE pp.is_active = TRUE
  AND pp.deleted_at IS NULL
  AND p.deleted_at IS NULL
  AND bp.deleted_at IS NULL
  AND a.deleted_at IS NULL
ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING;
