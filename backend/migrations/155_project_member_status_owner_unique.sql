-- Project member status and single-owner enforcement.
-- Owner is project-scoped and must be transferred, not multi-selected.

ALTER TABLE project_members
    ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active';

UPDATE project_members
SET status = 'active'
WHERE status IS NULL OR status = '';

WITH ranked_owners AS (
    SELECT
        id,
        ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY created_at ASC, id ASC) AS rn
    FROM project_members
    WHERE is_owner = TRUE
)
UPDATE project_members pm
SET is_owner = FALSE,
    updated_at = NOW()
FROM ranked_owners ro
WHERE pm.id = ro.id
  AND ro.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_project_members_single_owner
    ON project_members(project_id)
    WHERE is_owner = TRUE;

CREATE INDEX IF NOT EXISTS idx_project_members_project_status
    ON project_members(project_id, status);
