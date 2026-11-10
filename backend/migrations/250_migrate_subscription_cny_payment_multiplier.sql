-- Forward-only backfill; retain the retired setting so an older binary can be restored.
-- Valid USD/CNY rates, including an explicit zero, always take precedence.
DO $$
DECLARE
    canonical_raw TEXT;
    canonical_exists BOOLEAN;
    canonical_rate DOUBLE PRECISION;
    legacy_raw TEXT;
    legacy_multiplier DOUBLE PRECISION;
    migrated_rate DOUBLE PRECISION;
BEGIN
    SELECT value INTO canonical_raw
    FROM settings
    WHERE key = 'SUBSCRIPTION_USD_TO_CNY_RATE'
    FOR UPDATE;
    canonical_exists := FOUND;

    BEGIN
        canonical_rate := btrim(canonical_raw, E' \t\n\r\f\013')::DOUBLE PRECISION;
        IF canonical_rate >= 0 AND canonical_rate < 'Infinity'::DOUBLE PRECISION THEN
            RETURN;
        END IF;
    EXCEPTION WHEN invalid_text_representation OR numeric_value_out_of_range THEN
        NULL;
    END;

    SELECT value INTO legacy_raw
    FROM settings
    WHERE key = 'SUBSCRIPTION_CNY_PAYMENT_MULTIPLIER'
    FOR SHARE;
    IF legacy_raw IS NULL OR legacy_raw ~ '^[[:space:]]*$' THEN
        RETURN;
    END IF;

    BEGIN
        -- The retired Go parser rejected whitespace instead of trimming it.
        IF legacy_raw ~ '[[:space:]]' THEN
            legacy_multiplier := 1;
        ELSE
            legacy_multiplier := legacy_raw::DOUBLE PRECISION;
        END IF;
    EXCEPTION WHEN invalid_text_representation OR numeric_value_out_of_range THEN
        legacy_multiplier := 1;
    END;
    IF NOT (legacy_multiplier > 0 AND legacy_multiplier < 'Infinity'::DOUBLE PRECISION) THEN
        legacy_multiplier := 1;
    END IF;

    BEGIN
        -- Match decimal.Div's 16-place result followed by the former Round(6).
        migrated_rate := round(round(1 / legacy_multiplier::TEXT::NUMERIC, 16), 6)::DOUBLE PRECISION;
    EXCEPTION WHEN numeric_value_out_of_range THEN
        migrated_rate := 0;
    END;

    IF canonical_exists THEN
        UPDATE settings SET value = migrated_rate::TEXT, updated_at = NOW()
        WHERE key = 'SUBSCRIPTION_USD_TO_CNY_RATE';
    ELSE
        INSERT INTO settings (key, value, updated_at)
        VALUES ('SUBSCRIPTION_USD_TO_CNY_RATE', migrated_rate::TEXT, NOW())
        ON CONFLICT (key) DO NOTHING;
    END IF;
END $$;
