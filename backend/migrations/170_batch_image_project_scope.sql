ALTER TABLE batch_image_jobs
    ADD COLUMN IF NOT EXISTS project_id BIGINT;

UPDATE batch_image_jobs bij
SET project_id = COALESCE(
    (SELECT ak.project_id FROM api_keys ak WHERE ak.id = bij.api_key_id),
    (SELECT a.project_id FROM accounts a WHERE a.id = bij.account_id),
    (SELECT id FROM projects WHERE slug = 'default')
)
WHERE bij.project_id IS NULL;

ALTER TABLE batch_image_jobs
    ALTER COLUMN project_id SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'batch_image_jobs_project_id_fkey') THEN
        ALTER TABLE batch_image_jobs
            ADD CONSTRAINT batch_image_jobs_project_id_fkey
            FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS batch_image_jobs_project_user_created_at_idx
    ON batch_image_jobs (project_id, user_id, created_at);
