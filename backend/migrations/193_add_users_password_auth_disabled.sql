DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'users'
          AND column_name = 'password_auth_disabled'
    ) THEN
        ALTER TABLE users
            ADD COLUMN password_auth_disabled BOOLEAN;
    END IF;
END
$$;

-- NULL means the legacy row predates explicit password capability tracking.
-- OAuth signup source is used only as a passkey step-up fallback for those
-- rows; the first password write resolves the field to FALSE.
