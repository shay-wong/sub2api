-- Migrate active project administrators to the global administrator role.
-- Empty project scopes historically meant all permissions; __none__ meant none.
WITH active_admin_members AS (
    SELECT pm.user_id, COALESCE(pm.scopes, '[]'::jsonb) AS scopes
    FROM project_members pm
    JOIN projects p ON p.id = pm.project_id
    JOIN users u ON u.id = pm.user_id
    WHERE pm.role = 'admin'
      AND pm.status = 'active'
      AND p.status = 'active'
      AND p.deleted_at IS NULL
      AND u.deleted_at IS NULL
      AND u.role <> 'super_admin'
), permission_rows AS (
    SELECT aam.user_id, permission
    FROM active_admin_members aam
    CROSS JOIN LATERAL (
        SELECT permission
        FROM unnest(ARRAY[
            'admin.dashboard.read',
            'admin.ops.read',
            'admin.users.manage',
            'admin.groups.manage',
            'admin.proxies.manage',
            'admin.subscriptions.manage',
            'admin.accounts.write',
            'admin.usage.read'
        ]) AS permission
        WHERE jsonb_array_length(aam.scopes) = 0

        UNION ALL

        SELECT value
        FROM jsonb_array_elements_text(aam.scopes) AS value
        WHERE NOT (aam.scopes ? '__none__')
          AND value = ANY(ARRAY[
              'admin.dashboard.read',
              'admin.ops.read',
              'admin.users.manage',
              'admin.groups.manage',
              'admin.proxies.manage',
              'admin.subscriptions.manage',
              'admin.accounts.write',
              'admin.usage.read'
          ])
    ) permissions
), merged_permissions AS (
    SELECT user_id, jsonb_agg(DISTINCT permission ORDER BY permission) AS permissions
    FROM permission_rows
    GROUP BY user_id
), migrated_users AS (
    SELECT DISTINCT user_id FROM active_admin_members
)
UPDATE users u
SET role = 'admin',
    admin_permissions = COALESCE(mp.permissions, '[]'::jsonb),
    updated_at = NOW()
FROM migrated_users mu
LEFT JOIN merged_permissions mp ON mp.user_id = mu.user_id
WHERE u.id = mu.user_id
  AND u.role <> 'super_admin';
