-- Non-transactional migration: CREATE UNIQUE INDEX CONCURRENTLY cannot run in a transaction.
-- The operation marker is written in the same transaction as the copied account and its groups.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_accounts_duplicate_operation_id
    ON accounts ((extra ->> 'duplicate_operation_id'))
    WHERE deleted_at IS NULL
      AND extra ? 'duplicate_operation_id'
      AND NULLIF(extra ->> 'duplicate_operation_id', '') IS NOT NULL;
