//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBatchImageBillingRecoveryService_ReleasesStaleUnsubmittedHold(t *testing.T) {
	repo := newFakeBatchImageRepository()
	apiKeyID := int64(22)
	holdAmount := 0.5
	stale := &BatchImageJob{
		BatchID:       "imgbatch_stale_created",
		UserID:        11,
		ProjectID:     77,
		APIKeyID:      &apiKeyID,
		Status:        BatchImageJobStatusCreated,
		EstimatedCost: holdAmount,
		HoldAmount:    &holdAmount,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
	}
	activeProviderName := "providers/job"
	active := &BatchImageJob{
		BatchID:         "imgbatch_has_provider",
		UserID:          11,
		APIKeyID:        &apiKeyID,
		Status:          BatchImageJobStatusSubmitted,
		ProviderJobName: &activeProviderName,
		EstimatedCost:   holdAmount,
		HoldAmount:      &holdAmount,
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now().Add(-time.Hour),
	}
	repo.jobs[stale.BatchID] = stale
	repo.jobs[active.BatchID] = active
	billing := &fakeBatchImageBillingRepo{}
	authCache := &batchImageBillingRecoveryAuthCache{}
	svc := &BatchImageBillingRecoveryService{Repo: repo, Billing: billing, AuthCache: authCache, StaleAfter: time.Minute, Limit: 10}

	released, err := svc.ReleaseStaleUnsubmittedOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, released)
	require.Equal(t, BatchImageJobStatusFailed, repo.jobs[stale.BatchID].Status)
	require.Equal(t, "SUBMIT_STALE_BEFORE_PROVIDER", batchImageDerefString(repo.jobs[stale.BatchID].LastErrorCode))
	require.Equal(t, []int64{stale.ProjectID}, repo.transitionProjects)
	require.Len(t, billing.releases, 1)
	require.Equal(t, BatchImageReleaseRequestID(stale.BatchID), billing.releases[0].RequestID)
	require.Equal(t, []int64{stale.ProjectID}, billing.releaseProjects)
	require.Equal(t, []int64{stale.ProjectID}, authCache.userProjects)
	require.Equal(t, []int64{stale.UserID}, authCache.userIDs)
	require.Equal(t, BatchImageJobStatusSubmitted, repo.jobs[active.BatchID].Status)
}

type batchImageBillingRecoveryAuthCache struct {
	userIDs      []int64
	userProjects []int64
}

func (c *batchImageBillingRecoveryAuthCache) InvalidateAuthCacheByKey(context.Context, string) {}

func (c *batchImageBillingRecoveryAuthCache) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	c.userIDs = append(c.userIDs, userID)
	c.userProjects = append(c.userProjects, batchImageTestProjectIDFromContext(ctx))
}

func (c *batchImageBillingRecoveryAuthCache) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func (c *batchImageBillingRecoveryAuthCache) InvalidateAuthCacheByAPIKeyID(context.Context, int64) {}
