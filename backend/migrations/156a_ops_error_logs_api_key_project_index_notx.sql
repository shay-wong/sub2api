-- Support API-key project transfers and backfills without scanning the high-write
-- ops_error_logs table by api_key_id.
-- Non-transactional migration: CREATE INDEX CONCURRENTLY cannot run in a tx.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_error_logs_api_key_project
  ON ops_error_logs (api_key_id, project_id)
  WHERE api_key_id IS NOT NULL;
