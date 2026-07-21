-- Replace retained deleted API key credentials with irreversible SHA-256 digests.
-- Keep the deprecated key column empty so older application instances can finish
-- in-flight deletes during a rolling upgrade without persisting plaintext.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE deleted_api_key_audits
    ADD COLUMN IF NOT EXISTS key_digest VARCHAR(64);

CREATE OR REPLACE FUNCTION normalize_deleted_api_key_audit_digest()
RETURNS TRIGGER AS $$
BEGIN
    IF (NEW.key_digest IS NULL OR NEW.key_digest = '') AND NEW.key <> '' THEN
        NEW.key_digest := encode(sha256(convert_to(NEW.key, 'UTF8')), 'hex');
    END IF;
    NEW.key := '';
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS deleted_api_key_audit_digest_guard ON deleted_api_key_audits;
CREATE TRIGGER deleted_api_key_audit_digest_guard
    BEFORE INSERT OR UPDATE OF key, key_digest ON deleted_api_key_audits
    FOR EACH ROW
    EXECUTE FUNCTION normalize_deleted_api_key_audit_digest();

UPDATE deleted_api_key_audits
SET key_digest = encode(sha256(convert_to(key, 'UTF8')), 'hex'),
    key = ''
WHERE key_digest IS NULL OR key_digest = '' OR key <> '';

ALTER TABLE deleted_api_key_audits
    ALTER COLUMN key SET DEFAULT '',
    ALTER COLUMN key_digest SET NOT NULL;

DROP INDEX IF EXISTS deletedapikeyaudit_key;
CREATE INDEX IF NOT EXISTS deletedapikeyaudit_key_digest
    ON deleted_api_key_audits (key_digest);
