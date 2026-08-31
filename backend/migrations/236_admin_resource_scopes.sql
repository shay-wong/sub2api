CREATE TABLE IF NOT EXISTS admin_resource_scopes (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    mode VARCHAR(20) NOT NULL DEFAULT 'all',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT admin_resource_scopes_mode_check CHECK (mode IN ('all', 'restricted'))
);

CREATE TABLE IF NOT EXISTS admin_resource_bindings (
    user_id BIGINT NOT NULL REFERENCES admin_resource_scopes(user_id) ON DELETE CASCADE,
    resource_type VARCHAR(20) NOT NULL,
    resource_id BIGINT NOT NULL,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT admin_resource_bindings_type_check
        CHECK (resource_type IN ('group', 'account', 'proxy', 'subscription')),
    CONSTRAINT admin_resource_bindings_unique
        UNIQUE (user_id, resource_type, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_admin_resource_bindings_resource
    ON admin_resource_bindings(resource_type, resource_id);

INSERT INTO admin_resource_scopes (user_id, mode, created_at, updated_at)
SELECT id, 'all', NOW(), NOW()
FROM users
WHERE role = 'admin' AND deleted_at IS NULL
ON CONFLICT (user_id) DO NOTHING;
