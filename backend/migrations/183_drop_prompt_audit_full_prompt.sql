-- Raw prompts are restricted to request memory and short-lived Redis payloads.
ALTER TABLE prompt_audit_events
    DROP COLUMN IF EXISTS full_prompt;
