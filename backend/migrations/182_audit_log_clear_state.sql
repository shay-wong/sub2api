-- Persist the global clear cutoff independently from retained audit rows.
-- CACHE 1 keeps nextval allocation globally ordered across app instances.
-- Deployment prerequisite: stop every pre-182 application instance before
-- applying this migration, then start only instances that reserve audit IDs
-- before enqueueing. Old instances allocate IDs during COPY and cannot preserve
-- the clear boundary during a rolling deployment.
ALTER SEQUENCE audit_logs_id_seq CACHE 1 NO CYCLE;

CREATE TABLE IF NOT EXISTS audit_log_state (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    clear_id BIGINT NOT NULL DEFAULT 0
);

INSERT INTO audit_log_state (singleton, clear_id)
VALUES (TRUE, 0)
ON CONFLICT (singleton) DO NOTHING;
