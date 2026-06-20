-- Backfill API-key-owned logs after project transfers.
-- Earlier transfer code only moved api_keys.project_id, leaving usage/ops rows
-- under their previous project. Project dashboards and ops views read those
-- raw log project IDs, so they must follow the API key's owning project.

UPDATE usage_logs ul
SET project_id = ak.project_id
FROM api_keys ak
WHERE ul.api_key_id = ak.id
  AND ak.project_id IS NOT NULL
  AND ul.project_id IS DISTINCT FROM ak.project_id;

UPDATE ops_error_logs oel
SET project_id = ak.project_id
FROM api_keys ak
WHERE oel.api_key_id = ak.id
  AND ak.project_id IS NOT NULL
  AND oel.project_id IS DISTINCT FROM ak.project_id;
