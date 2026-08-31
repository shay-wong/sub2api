DROP TABLE IF EXISTS user_group_rate_limit_windows;

ALTER TABLE groups
    DROP COLUMN IF EXISTS rate_limit_5h;
