package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration112UsesIdempotentAddColumn(t *testing.T) {
	content, err := FS.ReadFile("112_add_payment_order_provider_key_snapshot.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS provider_key VARCHAR(30)")
	require.NotContains(t, sql, "ADD COLUMN provider_key VARCHAR(30);")
}

func TestMigration118DoesNotForceOverwriteAuthSourceGrantDefaults(t *testing.T) {
	content, err := FS.ReadFile("118_wechat_dual_mode_and_auth_source_defaults.sql")
	require.NoError(t, err)

	sql := string(content)
	require.NotContains(t, sql, "UPDATE settings")
	require.NotContains(t, sql, "SET value = 'false'")
	require.True(t, strings.Contains(sql, "ON CONFLICT (key) DO NOTHING"))
	require.Contains(t, sql, "THEN ''")
}

func TestAuthIdentityReportTypeWideningRunsBeforeLongReportWritersAndStillReconcilesAt121(t *testing.T) {
	preflightContent, err := FS.ReadFile("108a_widen_auth_identity_migration_report_type.sql")
	require.NoError(t, err)

	preflightSQL := string(preflightContent)
	require.Contains(t, preflightSQL, "ALTER TABLE auth_identity_migration_reports")
	require.Contains(t, preflightSQL, "ALTER COLUMN report_type TYPE VARCHAR(80)")

	content, err := FS.ReadFile("109_auth_identity_compat_backfill.sql")
	require.NoError(t, err)

	sql := string(content)
	require.NotContains(t, sql, "ALTER TABLE auth_identity_migration_reports")

	followupContent, err := FS.ReadFile("121_auth_identity_migration_report_type_widen.sql")
	require.NoError(t, err)

	followupSQL := string(followupContent)
	require.Contains(t, followupSQL, "ALTER TABLE auth_identity_migration_reports")
	require.Contains(t, followupSQL, "ALTER COLUMN report_type TYPE VARCHAR(80)")
}

func TestMigration119DefersPaymentIndexRolloutToOnlineFollowup(t *testing.T) {
	content, err := FS.ReadFile("119_enforce_payment_orders_out_trade_no_unique.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "120_enforce_payment_orders_out_trade_no_unique_notx.sql")
	require.Contains(t, sql, "NULL;")
	require.NotContains(t, sql, "CREATE UNIQUE INDEX")
	require.NotContains(t, sql, "DROP INDEX")

	followupContent, err := FS.ReadFile("120_enforce_payment_orders_out_trade_no_unique_notx.sql")
	require.NoError(t, err)

	followupSQL := string(followupContent)
	require.Contains(t, followupSQL, "explicit duplicate out_trade_no precheck")
	require.Contains(t, followupSQL, "stale invalid paymentorder_out_trade_no_unique index")
	require.Contains(t, followupSQL, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS paymentorder_out_trade_no_unique")
	require.NotContains(t, followupSQL, "DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no_unique")
	require.Contains(t, followupSQL, "DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no")
	require.Contains(t, followupSQL, "WHERE out_trade_no <> ''")

	alignmentContent, err := FS.ReadFile("120a_align_payment_orders_out_trade_no_index_name.sql")
	require.NoError(t, err)

	alignmentSQL := string(alignmentContent)
	require.Contains(t, alignmentSQL, "paymentorder_out_trade_no_unique")
	require.Contains(t, alignmentSQL, "RENAME TO paymentorder_out_trade_no")
}

func TestMigration110SeedsAuthSourceSignupGrantsDisabledByDefault(t *testing.T) {
	content, err := FS.ReadFile("110_pending_auth_and_provider_default_grants.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "('auth_source_default_email_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_linuxdo_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_oidc_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_wechat_grant_on_signup', 'false')")
	require.NotContains(t, sql, "('auth_source_default_email_grant_on_signup', 'true')")
}

func TestMigration122ScrubsPendingOAuthCompletionTokensAtRest(t *testing.T) {
	content, err := FS.ReadFile("122_pending_auth_completion_token_cleanup.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "UPDATE pending_auth_sessions")
	require.Contains(t, sql, "completion_response")
	require.Contains(t, sql, "access_token")
	require.Contains(t, sql, "refresh_token")
	require.Contains(t, sql, "expires_in")
	require.Contains(t, sql, "token_type")
}

func TestMigration123BackfillsLegacyAuthSourceGrantDefaultsSafely(t *testing.T) {
	content, err := FS.ReadFile("123_fix_legacy_auth_source_grant_on_signup_defaults.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "110_pending_auth_and_provider_default_grants.sql")
	require.Contains(t, sql, "schema_migrations")
	require.Contains(t, sql, "updated_at")
	require.Contains(t, sql, "'_grant_on_signup'")
	require.Contains(t, sql, "value = 'false'")
	require.Contains(t, sql, "auth_identity_migration_reports")
}

func TestMigration124BackfillsLegacyOIDCSecurityFlagsSafely(t *testing.T) {
	content, err := FS.ReadFile("124_backfill_legacy_oidc_security_flags.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "oidc_connect_use_pkce")
	require.Contains(t, sql, "oidc_connect_validate_id_token")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
	require.Contains(t, sql, "oidc_connect_enabled")
	require.Contains(t, sql, "'false'")
}

func TestMigration134AddsAffiliateLedgerAuditFieldsWithoutJSONCast(t *testing.T) {
	content, err := FS.ReadFile("134_affiliate_ledger_audit_snapshots.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS source_order_id BIGINT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS balance_after DECIMAL(20,8)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS aff_quota_after DECIMAL(20,8)")
	require.Contains(t, sql, "substring(")
	require.Contains(t, sql, `"rebateAmount"`)
	require.Contains(t, sql, "COUNT(*) OVER (PARTITION BY ra.order_id) AS order_match_count")
	require.Contains(t, sql, "COUNT(*) OVER (PARTITION BY ual.id) AS ledger_match_count")
	require.NotContains(t, sql, "detail::jsonb")
}

func TestMigration135AllowsGitHubAndGoogleAuthProviders(t *testing.T) {
	content, err := FS.ReadFile("135_allow_email_oauth_provider_types.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "users_signup_source_check")
	require.Contains(t, sql, "auth_identities_provider_type_check")
	require.Contains(t, sql, "auth_identity_channels_provider_type_check")
	require.Contains(t, sql, "pending_auth_sessions_provider_type_check")
	require.Contains(t, sql, "'github'")
	require.Contains(t, sql, "'google'")
}

func TestMigration151AddsAccountAutoPauseExpiryPartialIndex(t *testing.T) {
	content, err := FS.ReadFile("151_account_autopause_expiry_index_notx.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_autopause_expiry_due")
	require.Contains(t, sql, "ON accounts (expires_at)")
	require.Contains(t, sql, "WHERE deleted_at IS NULL")
	require.Contains(t, sql, "schedulable = TRUE")
	require.Contains(t, sql, "auto_pause_on_expired = TRUE")
	require.Contains(t, sql, "expires_at IS NOT NULL")
}

func TestMigration154CreatesDefaultProjectAndDemotesLegacyOperators(t *testing.T) {
	content, err := FS.ReadFile("154_project_isolation_default_project.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS projects")
	require.Contains(t, sql, "VALUES ('默认项目', 'default'")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS project_members")
	require.Contains(t, sql, "Legacy operator permissions are intentionally not migrated")
	require.Contains(t, sql, "SET role = 'super_admin'")
	require.Contains(t, sql, "SET role = 'user'")
	require.Contains(t, sql, "WHERE id IN (SELECT id FROM legacy_user_roles WHERE role = 'operator')")
	require.Contains(t, sql, "WHEN lur.role = 'admin' OR u.role = 'super_admin' THEN 'admin'")
	require.Contains(t, sql, "ELSE 'user'")
	require.NotContains(t, sql, "THEN 'operator'")

	for _, table := range []string{"groups", "accounts", "api_keys", "usage_logs"} {
		require.Contains(t, sql, "ALTER TABLE "+table+"\n    ADD COLUMN IF NOT EXISTS project_id BIGINT")
		require.Contains(t, sql, "UPDATE "+table+"\nSET project_id = (SELECT id FROM projects WHERE slug = 'default')")
		require.Contains(t, sql, "ALTER TABLE "+table+"\n    ALTER COLUMN project_id SET NOT NULL")
	}
	for _, table := range []string{"ops_error_logs", "ops_system_metrics", "ops_metrics_hourly", "ops_metrics_daily", "ops_alert_events", "ops_system_logs"} {
		require.Contains(t, sql, "ALTER TABLE "+table+"\n    ADD COLUMN IF NOT EXISTS project_id BIGINT")
		require.Contains(t, sql, table+"_project_id_fkey")
	}
}

func TestMigration155EnforcesSingleProjectOwnerAndMemberStatus(t *testing.T) {
	content, err := FS.ReadFile("155_project_member_status_owner_unique.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active'")
	require.Contains(t, sql, "ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY created_at ASC, id ASC)")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_project_members_single_owner")
	require.Contains(t, sql, "ON project_members(project_id)")
	require.Contains(t, sql, "WHERE is_owner = TRUE")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_project_members_project_status")
	require.Contains(t, sql, "ON project_members(project_id, status)")
}

func TestMigration156AddsProjectProfilesWithUnrestrictedModeAndResourceBindings(t *testing.T) {
	content, err := FS.ReadFile("156_project_profiles_bindings.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS project_profiles")
	require.Contains(t, sql, "mode VARCHAR(20) NOT NULL DEFAULT 'restricted'")
	require.Contains(t, sql, "CHECK (mode IN ('restricted', 'unrestricted'))")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_project_profiles_one_active")
	require.Contains(t, sql, "WHERE is_active = TRUE AND deleted_at IS NULL")

	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS project_profile_bindings")
	require.Contains(t, sql, "CHECK (resource_type IN ('user', 'group', 'account', 'subscription', 'api_key'))")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_project_profile_bindings_unique")
	require.Contains(t, sql, "ON project_profile_bindings(project_profile_id, resource_type, resource_id)")
	require.Contains(t, sql, "'Default active project application profile.'")
	require.Contains(t, sql, "pm.status = 'active'")

	for _, resourceType := range []string{"user", "group", "account", "api_key", "subscription"} {
		require.Contains(t, sql, "SELECT pp.id, '"+resourceType+"'")
	}
}

func TestMigration158BackfillsSystemLogsOnlyFromUnambiguousProjectMatches(t *testing.T) {
	content, err := FS.ReadFile("158_backfill_system_log_project_scope.sql")
	require.NoError(t, err)

	sql := string(content)
	require.NotContains(t, sql, "DISTINCT ON", "project backfill must not pick an arbitrary latest project")
	require.Contains(t, sql, "WITH candidate_projects AS (")
	require.Contains(t, sql, "UNION ALL")
	require.Contains(t, sql, "unambiguous_matches AS (")
	require.Contains(t, sql, "FROM candidate_projects")
	require.Contains(t, sql, "HAVING COUNT(DISTINCT project_id) = 1")
	require.Equal(t, 2, strings.Count(sql, "UPDATE ops_system_logs sl\nSET project_id ="))
	require.Contains(t, sql, "JOIN usage_logs ul\n      ON ul.request_id = sl.request_id")
	require.Contains(t, sql, "JOIN ops_error_logs oel\n      ON oel.request_id = sl.request_id")
	require.Contains(t, sql, "JOIN ops_error_logs oel\n      ON oel.client_request_id = sl.client_request_id")
	require.Contains(t, sql, "NOT EXISTS (\n      SELECT 1\n      FROM usage_logs ul")
}

func TestMigration159ScopesProxiesToProjectsAndProfiles(t *testing.T) {
	content, err := FS.ReadFile("159_project_scoped_proxies.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE proxies\n    ADD COLUMN IF NOT EXISTS project_id BIGINT")
	require.Contains(t, sql, "WHERE slug = 'default'")
	require.Contains(t, sql, "ALTER TABLE proxies\n    ALTER COLUMN project_id SET NOT NULL")
	require.Contains(t, sql, "ADD CONSTRAINT proxies_projects_proxies")
	require.Contains(t, sql, "FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT")
	require.Contains(t, sql, "CHECK (resource_type IN ('user', 'group', 'account', 'proxy', 'subscription', 'api_key'))")
	require.Contains(t, sql, "SELECT DISTINCT pp.id, 'proxy', p.id")
	require.Contains(t, sql, "JOIN proxies p ON p.project_id = pp.project_id")
	require.Contains(t, sql, "JOIN accounts a ON a.proxy_id = p.id")
	require.Contains(t, sql, "SELECT DISTINCT pp.id, 'proxy', bp.id")
	require.Contains(t, sql, "JOIN proxies bp ON bp.id = p.backup_proxy_id")
	require.Contains(t, sql, "ON CONFLICT (project_profile_id, resource_type, resource_id) DO NOTHING")
}
