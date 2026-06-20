-- Backfill system logs into the same project space as correlated request data.
-- System logs created before project-aware access logging often fell back to
-- the default project even when usage/error logs or accounts already identify
-- the request's actual project.

WITH candidate_projects AS (
    SELECT
        sl.id AS system_log_id,
        ul.project_id
    FROM ops_system_logs sl
    JOIN usage_logs ul
      ON ul.request_id = sl.request_id
    WHERE sl.request_id IS NOT NULL
      AND sl.request_id <> ''
      AND ul.project_id IS NOT NULL

    UNION ALL

    SELECT
        sl.id AS system_log_id,
        oel.project_id
    FROM ops_system_logs sl
    JOIN ops_error_logs oel
      ON oel.request_id = sl.request_id
    WHERE sl.request_id IS NOT NULL
      AND sl.request_id <> ''
      AND oel.project_id IS NOT NULL

    UNION ALL

    SELECT
        sl.id AS system_log_id,
        oel.project_id
    FROM ops_system_logs sl
    JOIN ops_error_logs oel
      ON oel.client_request_id = sl.client_request_id
    WHERE sl.client_request_id IS NOT NULL
      AND sl.client_request_id <> ''
      AND oel.project_id IS NOT NULL
),
unambiguous_matches AS (
    SELECT
        system_log_id,
        MIN(project_id) AS project_id
    FROM candidate_projects
    GROUP BY system_log_id
    HAVING COUNT(DISTINCT project_id) = 1
)
UPDATE ops_system_logs sl
SET project_id = m.project_id
FROM unambiguous_matches m
WHERE sl.id = m.system_log_id
  AND sl.project_id IS DISTINCT FROM m.project_id;

UPDATE ops_system_logs sl
SET project_id = a.project_id
FROM accounts a
WHERE sl.account_id = a.id
  AND a.project_id IS NOT NULL
  AND sl.project_id IS DISTINCT FROM a.project_id
  AND NOT EXISTS (
      SELECT 1
      FROM usage_logs ul
      WHERE sl.request_id IS NOT NULL
        AND sl.request_id <> ''
        AND ul.request_id = sl.request_id
        AND ul.project_id IS NOT NULL
  )
  AND NOT EXISTS (
      SELECT 1
      FROM ops_error_logs oel
      WHERE sl.request_id IS NOT NULL
        AND sl.request_id <> ''
        AND oel.request_id = sl.request_id
        AND oel.project_id IS NOT NULL
  )
  AND NOT EXISTS (
      SELECT 1
      FROM ops_error_logs oel
      WHERE sl.client_request_id IS NOT NULL
        AND sl.client_request_id <> ''
        AND oel.client_request_id = sl.client_request_id
        AND oel.project_id IS NOT NULL
  );
