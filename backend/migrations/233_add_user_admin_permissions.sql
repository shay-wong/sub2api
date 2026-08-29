-- Expand: store administrator permissions directly on the user.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS admin_permissions JSONB NOT NULL DEFAULT '[]'::jsonb;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'users_admin_permissions_array'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_admin_permissions_array
            CHECK (jsonb_typeof(admin_permissions) = 'array');
    END IF;
END $$;
