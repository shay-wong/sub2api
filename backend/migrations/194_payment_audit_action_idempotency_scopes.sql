-- Expand phase: keep the historical all-action unique index so binaries from
-- the previous release can continue using ON CONFLICT (order_id, action).
-- A later fork release may drop it only after all old binaries are drained.
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_audit_logs_order_action_uniq
ON payment_audit_logs(order_id, action);

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY order_id
               ORDER BY
                   CASE action WHEN 'AFFILIATE_REBATE_APPLIED' THEN 0 ELSE 1 END,
                   created_at DESC,
                   id DESC
           ) AS rn
    FROM payment_audit_logs
    WHERE action IN ('AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED')
)
DELETE FROM payment_audit_logs p
USING ranked r
WHERE p.id = r.id
  AND r.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_audit_logs_refund_state_uniq
ON payment_audit_logs(order_id, action)
WHERE action IN ('REFUND_PENDING', 'REFUND_ROLLBACK_FAILED');

CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_audit_logs_affiliate_rebate_uniq
ON payment_audit_logs(order_id)
WHERE action IN ('AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED');

CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_audit_logs_subscription_assignment_uniq
ON payment_audit_logs(order_id)
WHERE action = 'SUBSCRIPTION_ASSIGNED';
