//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDeletedAPIKeyDigestAttributionEndToEnd(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{})
	rawKey := fmt.Sprintf("sk-deleted-attribution-%d", time.Now().UnixNano())
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    rawKey,
		Name:   "deleted attribution",
	})

	t.Cleanup(func() {
		_, _ = integrationDB.Exec(`DELETE FROM deleted_api_key_audits WHERE api_key_id = $1`, apiKey.ID)
		_, _ = integrationDB.Exec(`DELETE FROM api_keys WHERE id = $1`, apiKey.ID)
		_, _ = integrationDB.Exec(`DELETE FROM users WHERE id = $1`, user.ID)
	})

	apiKeyRepo := NewAPIKeyRepository(client, integrationDB)
	require.NoError(t, apiKeyRepo.DeleteWithAudit(ctx, apiKey.ID))

	opsRepo := NewOpsRepository(integrationDB)
	attribution, err := opsRepo.LookupDeletedKeyAudit(ctx, rawKey)
	require.NoError(t, err)
	require.NotNil(t, attribution)
	require.Equal(t, user.ID, attribution.UserID)
	require.Equal(t, "deleted attribution", attribution.KeyName)

	missing, err := opsRepo.LookupDeletedKeyAudit(ctx, rawKey+"-wrong")
	require.NoError(t, err)
	require.Nil(t, missing)

	var retainedKey, keyDigest string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT key, key_digest FROM deleted_api_key_audits WHERE api_key_id = $1`,
		apiKey.ID,
	).Scan(&retainedKey, &keyDigest))
	require.Empty(t, retainedKey)
	require.Len(t, keyDigest, 64)
}
