-- Project application profiles and resource bindings.
-- Resources stay canonical; profiles decide which resources a project can see.

CREATE TABLE IF NOT EXISTS project_profiles (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    mode VARCHAR(20) NOT NULL DEFAULT 'restricted',
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT project_profiles_mode_check
        CHECK (mode IN ('restricted', 'unrestricted'))
);

CREATE INDEX IF NOT EXISTS idx_project_profiles_project_id
    ON project_profiles(project_id);
CREATE INDEX IF NOT EXISTS idx_project_profiles_project_active
    ON project_profiles(project_id, is_active);
CREATE INDEX IF NOT EXISTS idx_project_profiles_deleted_at
    ON project_profiles(deleted_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_project_profiles_one_active
    ON project_profiles(project_id)
    WHERE is_active = TRUE AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS project_profile_bindings (
    id BIGSERIAL PRIMARY KEY,
    project_profile_id BIGINT NOT NULL REFERENCES project_profiles(id) ON DELETE CASCADE,
    resource_type VARCHAR(30) NOT NULL,
    resource_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT project_profile_bindings_resource_type_check
        CHECK (resource_type IN ('user', 'group', 'account', 'subscription', 'api_key'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_project_profile_bindings_unique
    ON project_profile_bindings(project_profile_id, resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_project_profile_bindings_resource
    ON project_profile_bindings(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_project_profile_bindings_profile_type
    ON project_profile_bindings(project_profile_id, resource_type);

INSERT INTO project_profiles (project_id, name, description, mode, is_active, created_at, updated_at)
SELECT
    p.id,
    '默认配置',
    'Default active project application profile.',
    'restricted',
    TRUE,
    NOW(),
    NOW()
FROM projects p
WHERE p.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM project_profiles pp
      WHERE pp.project_id = p.id
        AND pp.deleted_at IS NULL
  );

INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
SELECT pp.id, 'user', pm.user_id, NOW(), '{}'::jsonb
FROM project_profiles pp
JOIN project_members pm ON pm.project_id = pp.project_id
WHERE pp.is_active = TRUE
  AND pp.deleted_at IS NULL
  AND pm.status = 'active'
ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING;

INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
SELECT pp.id, 'group', g.id, NOW(), '{}'::jsonb
FROM project_profiles pp
JOIN groups g ON g.project_id = pp.project_id
WHERE pp.is_active = TRUE
  AND pp.deleted_at IS NULL
  AND g.deleted_at IS NULL
ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING;

INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
SELECT pp.id, 'account', a.id, NOW(), '{}'::jsonb
FROM project_profiles pp
JOIN accounts a ON a.project_id = pp.project_id
WHERE pp.is_active = TRUE
  AND pp.deleted_at IS NULL
  AND a.deleted_at IS NULL
ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING;

INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
SELECT pp.id, 'api_key', ak.id, NOW(), '{}'::jsonb
FROM project_profiles pp
JOIN api_keys ak ON ak.project_id = pp.project_id
WHERE pp.is_active = TRUE
  AND pp.deleted_at IS NULL
  AND ak.deleted_at IS NULL
ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING;

INSERT INTO project_profile_bindings (project_profile_id, resource_type, resource_id, created_at, metadata)
SELECT pp.id, 'subscription', us.id, NOW(), '{}'::jsonb
FROM project_profiles pp
JOIN groups g ON g.project_id = pp.project_id
JOIN user_subscriptions us ON us.group_id = g.id
WHERE pp.is_active = TRUE
  AND pp.deleted_at IS NULL
  AND g.deleted_at IS NULL
  AND us.deleted_at IS NULL
ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING;
