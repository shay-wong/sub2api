//go:build integration

package repository

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newBatchImageRepositoryWithSQL(sqlq batchImageSQLExecutor) *batchImageRepository {
	return &batchImageRepository{sql: sqlq}
}

func TestBatchImageRepository_CreateJobAndDuplicates(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "create")

	job, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:       batchID,
		UserID:        1001,
		Provider:      service.BatchImageProviderGeminiAPI,
		Model:         "gemini-2.5-flash-image",
		ItemCount:     2,
		EstimatedCost: 0.02,
	})
	require.NoError(t, err)
	require.Equal(t, batchID, job.BatchID)
	require.Equal(t, service.BatchImageJobStatusCreated, job.Status)
	require.Equal(t, "USD", job.Currency)

	_, err = repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderGeminiAPI,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageJobExists))
}

func TestBatchImageRepository_ProjectScopeUsesJobProjectID(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	projectA := createBatchImageTestProject(t, tx, "batch-image-a-"+batchImageSafeTestIDSegment(t.Name(), 28))
	projectB := createBatchImageTestProject(t, tx, "batch-image-b-"+batchImageSafeTestIDSegment(t.Name(), 28))
	apiKeyID := int64(4242)
	otherAPIKeyID := int64(4243)
	userID := int64(1001)
	idempotencyKey := batchImageTestStringPtr("idem-" + batchImageSafeTestIDSegment(t.Name(), 48))

	jobA, err := repo.CreateBatchImageJob(service.WithProjectID(ctx, projectA), service.CreateBatchImageJobParams{
		BatchID:        batchImageTestID(t, "scope-a"),
		UserID:         userID,
		APIKeyID:       &apiKeyID,
		Provider:       service.BatchImageProviderGeminiAPI,
		Model:          "gemini-2.5-flash-image",
		ItemCount:      1,
		IdempotencyKey: idempotencyKey,
	})
	require.NoError(t, err)
	require.Equal(t, projectA, jobA.ProjectID)

	jobB, err := repo.CreateBatchImageJob(service.WithProjectID(ctx, projectB), service.CreateBatchImageJobParams{
		BatchID:        batchImageTestID(t, "scope-b"),
		UserID:         userID,
		APIKeyID:       &apiKeyID,
		Provider:       service.BatchImageProviderGeminiAPI,
		Model:          "gemini-2.5-flash-image",
		ItemCount:      1,
		IdempotencyKey: idempotencyKey,
	})
	require.NoError(t, err)
	require.Equal(t, projectB, jobB.ProjectID)

	jobAOtherKey, err := repo.CreateBatchImageJob(service.WithProjectID(ctx, projectA), service.CreateBatchImageJobParams{
		BatchID:   batchImageTestID(t, "scope-a-other-key"),
		UserID:    userID,
		APIKeyID:  &otherAPIKeyID,
		Provider:  service.BatchImageProviderGeminiAPI,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.NoError(t, err)
	require.Equal(t, projectA, jobAOtherKey.ProjectID)

	gotA, err := repo.GetBatchImageJobByIdempotencyKey(service.WithProjectID(ctx, projectA), userID, apiKeyID, *idempotencyKey)
	require.NoError(t, err)
	require.Equal(t, jobA.BatchID, gotA.BatchID)

	gotB, err := repo.GetBatchImageJobByIdempotencyKey(service.WithProjectID(ctx, projectB), userID, apiKeyID, *idempotencyKey)
	require.NoError(t, err)
	require.Equal(t, jobB.BatchID, gotB.BatchID)

	_, err = repo.GetBatchImageJobByBatchIDForOwner(service.WithProjectID(ctx, projectA), userID, apiKeyID, jobB.BatchID)
	require.ErrorIs(t, err, service.ErrBatchImageJobNotFound)

	jobsA, err := repo.ListBatchImageJobsForOwner(service.WithProjectID(ctx, projectA), userID, apiKeyID, service.BatchImageJobFilter{Limit: 20})
	require.NoError(t, err)
	require.Len(t, jobsA, 1)
	require.Equal(t, jobA.BatchID, jobsA[0].BatchID)

	jobsB, err := repo.ListBatchImageJobsForOwner(service.WithProjectID(ctx, projectB), userID, apiKeyID, service.BatchImageJobFilter{Limit: 20})
	require.NoError(t, err)
	require.Len(t, jobsB, 1)
	require.Equal(t, jobB.BatchID, jobsB[0].BatchID)

	allProjectA, err := repo.ListBatchImageJobsForUser(service.WithProjectID(ctx, projectA), userID, service.BatchImageJobFilter{Limit: 20})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{jobA.BatchID, jobAOtherKey.BatchID}, batchImageTestJobIDs(allProjectA))

	allProjectB, err := repo.ListBatchImageJobsForUser(service.WithProjectID(ctx, projectB), userID, service.BatchImageJobFilter{Limit: 20})
	require.NoError(t, err)
	require.Equal(t, []string{jobB.BatchID}, batchImageTestJobIDs(allProjectB))
}

func TestBatchImageRepository_InvalidProvider(t *testing.T) {
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)

	_, err := repo.CreateBatchImageJob(context.Background(), service.CreateBatchImageJobParams{
		BatchID:   batchImageTestID(t, "provider"),
		UserID:    1001,
		Provider:  "unknown",
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageInvalidProvider))
}

func TestBatchImageRepository_TransitionIncrementsVersionAndEvents(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "transition")
	now := time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC)

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderVertex,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.NoError(t, err)

	err = repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusUploading, service.BatchImageTransitionOptions{
		EventType:    "status_changed",
		EventPayload: map[string]any{"to": service.BatchImageJobStatusUploading},
		Now:          &now,
	})
	require.NoError(t, err)

	job, err := repo.GetBatchImageJobByBatchID(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, service.BatchImageJobStatusUploading, job.Status)
	require.Equal(t, 1, job.Version)

	var eventCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM batch_image_events WHERE job_id = $1 AND event_type = 'status_changed'`, batchID).Scan(&eventCount)
	require.NoError(t, err)
	require.Equal(t, 1, eventCount)
}

func TestBatchImageRepository_InvalidTransition(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "invalid-transition")

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderGeminiAPI,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.NoError(t, err)

	err = repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusRunning, service.BatchImageTransitionOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageInvalidTransition))
}

func TestBatchImageRepository_TouchBatchImageJobSubmittingRefreshesOnlyUnsubmitted(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	oldTime := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	uploadingID := batchImageTestID(t, "touch-uploading")
	submittedID := batchImageTestID(t, "touch-submitted")

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:       uploadingID,
		UserID:        1001,
		Provider:      service.BatchImageProviderGeminiAPI,
		Model:         "gemini-image",
		Status:        service.BatchImageJobStatusUploading,
		ItemCount:     1,
		EstimatedCost: 0.2,
	})
	require.NoError(t, err)
	_, err = repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:       submittedID,
		UserID:        1001,
		Provider:      service.BatchImageProviderGeminiAPI,
		Model:         "gemini-image",
		Status:        service.BatchImageJobStatusSubmitted,
		ItemCount:     1,
		EstimatedCost: 0.2,
	})
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `UPDATE batch_image_jobs SET updated_at = $1 WHERE batch_id IN ($2, $3)`, oldTime, uploadingID, submittedID)
	require.NoError(t, err)

	require.NoError(t, repo.TouchBatchImageJobSubmitting(ctx, uploadingID))
	require.NoError(t, repo.TouchBatchImageJobSubmitting(ctx, submittedID))

	var uploadingUpdated, submittedUpdated time.Time
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT updated_at FROM batch_image_jobs WHERE batch_id = $1`, uploadingID).Scan(&uploadingUpdated))
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT updated_at FROM batch_image_jobs WHERE batch_id = $1`, submittedID).Scan(&submittedUpdated))
	require.True(t, uploadingUpdated.After(oldTime), "uploading job should be refreshed")
	require.WithinDuration(t, oldTime, submittedUpdated, time.Millisecond, "submitted job should not be refreshed")
}

func TestBatchImageRepository_FailStaleUnsubmittedBatchImageJobUsesAtomicGuards(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	oldTime := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	cutoff := time.Now().Add(-time.Minute)
	staleID := batchImageTestID(t, "fail-stale")
	freshID := batchImageTestID(t, "fail-fresh")
	withProviderID := batchImageTestID(t, "fail-provider")
	providerJob := "providers/job"

	for _, params := range []service.CreateBatchImageJobParams{
		{BatchID: staleID, Status: service.BatchImageJobStatusCreated},
		{BatchID: freshID, Status: service.BatchImageJobStatusUploading},
		{BatchID: withProviderID, Status: service.BatchImageJobStatusUploading, ProviderJobName: &providerJob},
	} {
		params.UserID = 1001
		params.Provider = service.BatchImageProviderGeminiAPI
		params.Model = "gemini-image"
		params.ItemCount = 1
		params.EstimatedCost = 0.2
		_, err := repo.CreateBatchImageJob(ctx, params)
		require.NoError(t, err)
	}
	_, err := tx.ExecContext(ctx, `UPDATE batch_image_jobs SET updated_at = $1 WHERE batch_id IN ($2, $3)`, oldTime, staleID, withProviderID)
	require.NoError(t, err)

	applied, err := repo.FailStaleUnsubmittedBatchImageJob(ctx, staleID, cutoff, service.BatchImageErrorSubmitStaleBeforeProvider, "stale")
	require.NoError(t, err)
	require.True(t, applied)
	stale, err := repo.GetBatchImageJobByBatchID(ctx, staleID)
	require.NoError(t, err)
	require.Equal(t, service.BatchImageJobStatusFailed, stale.Status)
	require.Equal(t, service.BatchImageErrorSubmitStaleBeforeProvider, batchImageDerefTest(stale.LastErrorCode))

	applied, err = repo.FailStaleUnsubmittedBatchImageJob(ctx, freshID, cutoff, service.BatchImageErrorSubmitStaleBeforeProvider, "stale")
	require.NoError(t, err)
	require.False(t, applied)
	fresh, err := repo.GetBatchImageJobByBatchID(ctx, freshID)
	require.NoError(t, err)
	require.Equal(t, service.BatchImageJobStatusUploading, fresh.Status)

	applied, err = repo.FailStaleUnsubmittedBatchImageJob(ctx, withProviderID, cutoff, service.BatchImageErrorSubmitStaleBeforeProvider, "stale")
	require.NoError(t, err)
	require.False(t, applied)
	withProvider, err := repo.GetBatchImageJobByBatchID(ctx, withProviderID)
	require.NoError(t, err)
	require.Equal(t, service.BatchImageJobStatusUploading, withProvider.Status)
}

func TestBatchImageRepository_ListStaleUnsubmittedBatchImageJobsRetriesReleaseFailures(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	oldTime := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	cutoff := time.Now().Add(-time.Minute)
	holdAmount := 0.3
	staleID := batchImageTestID(t, "list-stale")
	releaseFailedID := batchImageTestID(t, "list-release-failed")
	providerReleaseFailedID := batchImageTestID(t, "list-provider-release-failed")
	submitStaleFailedID := batchImageTestID(t, "list-submit-stale-failed")
	recoveredID := batchImageTestID(t, "list-recovered")
	retryTime := time.Now().Add(-30 * time.Second).Truncate(time.Microsecond)
	providerJob := "providers/release-failed"

	for _, params := range []service.CreateBatchImageJobParams{
		{BatchID: staleID, Status: service.BatchImageJobStatusCreated},
		{BatchID: releaseFailedID, Status: service.BatchImageJobStatusFailed},
		{BatchID: providerReleaseFailedID, Status: service.BatchImageJobStatusFailed, ProviderJobName: &providerJob},
		{BatchID: submitStaleFailedID, Status: service.BatchImageJobStatusFailed},
		{BatchID: recoveredID, Status: service.BatchImageJobStatusFailed},
	} {
		params.UserID = 1001
		params.Provider = service.BatchImageProviderGeminiAPI
		params.Model = "gemini-image"
		params.ItemCount = 1
		params.EstimatedCost = holdAmount
		params.HoldAmount = &holdAmount
		_, err := repo.CreateBatchImageJob(ctx, params)
		require.NoError(t, err)
	}
	_, err := tx.ExecContext(ctx, `
UPDATE batch_image_jobs
SET updated_at = $1,
    last_error_code = CASE
      WHEN batch_id = $3 THEN $5
      WHEN batch_id = $4 THEN $6
      WHEN batch_id = $7 THEN $8
      WHEN batch_id = $9 THEN $5
      ELSE last_error_code
    END
WHERE batch_id IN ($2, $3, $4, $7, $9)`, oldTime, staleID, releaseFailedID, recoveredID, service.BatchImageErrorBillingReleaseFailed, service.BatchImageErrorBillingReleaseRecovered, submitStaleFailedID, service.BatchImageErrorSubmitStaleBeforeProvider, providerReleaseFailedID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `UPDATE batch_image_jobs SET updated_at = $1 WHERE batch_id = $2`, retryTime, releaseFailedID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `UPDATE batch_image_jobs SET updated_at = $1 WHERE batch_id = $2`, retryTime.Add(time.Second), submitStaleFailedID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `UPDATE batch_image_jobs SET updated_at = $1 WHERE batch_id = $2`, retryTime.Add(2*time.Second), providerReleaseFailedID)
	require.NoError(t, err)

	jobs, err := repo.ListStaleUnsubmittedBatchImageJobs(ctx, cutoff, 10)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{staleID, releaseFailedID, providerReleaseFailedID, submitStaleFailedID}, batchImageTestJobIDs(jobs))
	require.Equal(t, releaseFailedID, jobs[0].BatchID)

	jobs, err = repo.ListStaleUnsubmittedBatchImageJobs(ctx, cutoff, 1)
	require.NoError(t, err)
	require.Equal(t, []string{releaseFailedID}, batchImageTestJobIDs(jobs))

	require.NoError(t, repo.MarkBatchImageBillingReleaseRecovered(ctx, releaseFailedID))
	jobs, err = repo.ListStaleUnsubmittedBatchImageJobs(ctx, cutoff, 10)
	require.NoError(t, err)
	require.Equal(t, submitStaleFailedID, jobs[0].BatchID)
	require.ElementsMatch(t, []string{staleID, providerReleaseFailedID, submitStaleFailedID}, batchImageTestJobIDs(jobs))

	require.NoError(t, repo.MarkBatchImageBillingReleaseRecovered(ctx, submitStaleFailedID))
	require.NoError(t, repo.MarkBatchImageBillingReleaseRecovered(ctx, providerReleaseFailedID))
	jobs, err = repo.ListStaleUnsubmittedBatchImageJobs(ctx, cutoff, 10)
	require.NoError(t, err)
	require.Equal(t, []string{staleID}, batchImageTestJobIDs(jobs))

	var eventCount int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM batch_image_events WHERE job_id = $1 AND event_type = 'billing_hold_release_recovered'`, releaseFailedID).Scan(&eventCount))
	require.Equal(t, 1, eventCount)
}

func TestBatchImageRepository_TerminalStatusCannotMoveBack(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "terminal")

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderGeminiAPI,
		Model:     "gemini-2.5-flash-image",
		Status:    service.BatchImageJobStatusCompleted,
		ItemCount: 1,
	})
	require.NoError(t, err)

	err = repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusRunning, service.BatchImageTransitionOptions{})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageInvalidTransition))
}

func TestBatchImageRepository_ItemCustomIDUniqueness(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	firstBatchID := batchImageTestID(t, "items-a")
	secondBatchID := batchImageTestID(t, "items-b")

	for _, batchID := range []string{firstBatchID, secondBatchID} {
		_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
			BatchID:   batchID,
			UserID:    1001,
			Provider:  service.BatchImageProviderGeminiAPI,
			Model:     "gemini-2.5-flash-image",
			ItemCount: 1,
		})
		require.NoError(t, err)
	}

	_, err := repo.CreateBatchImageItem(ctx, service.CreateBatchImageItemParams{
		JobID:      firstBatchID,
		CustomID:   "line-1",
		Status:     service.BatchImageItemStatusSuccess,
		ImageCount: 1,
	})
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `SAVEPOINT batch_image_duplicate_item`)
	require.NoError(t, err)
	_, err = repo.CreateBatchImageItem(ctx, service.CreateBatchImageItemParams{
		JobID:    firstBatchID,
		CustomID: "line-1",
		Status:   service.BatchImageItemStatusFailed,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBatchImageItemExists))
	_, rollbackErr := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT batch_image_duplicate_item`)
	require.NoError(t, rollbackErr)

	_, err = repo.CreateBatchImageItem(ctx, service.CreateBatchImageItemParams{
		JobID:      secondBatchID,
		CustomID:   "line-1",
		Status:     service.BatchImageItemStatusSuccess,
		ImageCount: 1,
	})
	require.NoError(t, err)

	items, err := repo.ListBatchImageItems(ctx, firstBatchID, service.BatchImageItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestBatchImageRepository_ReplaceBatchImageItemsForJob(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "replace-items")
	lineOne := 1
	lineTwo := 2

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderGeminiAPI,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 2,
	})
	require.NoError(t, err)

	// 非 indexing 状态不允许重建 item 表：防止锁过期后掉队的 worker
	// 重写已完成/已结算 job 的条目。
	err = repo.ReplaceBatchImageItemsForJob(ctx, batchID, []service.CreateBatchImageItemParams{
		{CustomID: "old", Status: service.BatchImageItemStatusSuccess, SourceLineNumber: &lineOne, ImageCount: 1},
	}, service.BatchImageCounts{SuccessCount: 1})
	require.ErrorIs(t, err, service.ErrBatchImageIndexStateConflict)

	require.NoError(t, repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusSubmitted, service.BatchImageTransitionOptions{}))
	require.NoError(t, repo.TransitionBatchImageJobStatus(ctx, batchID, service.BatchImageJobStatusIndexing, service.BatchImageTransitionOptions{}))

	err = repo.ReplaceBatchImageItemsForJob(ctx, batchID, []service.CreateBatchImageItemParams{
		{CustomID: "old", Status: service.BatchImageItemStatusSuccess, SourceLineNumber: &lineOne, ImageCount: 1},
	}, service.BatchImageCounts{SuccessCount: 1})
	require.NoError(t, err)

	err = repo.ReplaceBatchImageItemsForJob(ctx, batchID, []service.CreateBatchImageItemParams{
		{CustomID: "new-ok", Status: service.BatchImageItemStatusSuccess, SourceLineNumber: &lineOne, ImageCount: 1},
		{CustomID: "new-fail", Status: service.BatchImageItemStatusFailed, SourceLineNumber: &lineTwo, ErrorCode: batchImageTestStringPtr("SAFETY_BLOCKED")},
	}, service.BatchImageCounts{SuccessCount: 1, FailCount: 1})
	require.NoError(t, err)

	items, err := repo.ListBatchImageItems(ctx, batchID, service.BatchImageItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "new-ok", items[0].CustomID)
	require.Equal(t, "new-fail", items[1].CustomID)

	job, err := repo.GetBatchImageJobByBatchID(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, 1, job.SuccessCount)
	require.Equal(t, 1, job.FailCount)
}

func TestBatchImageRepository_MarkBatchImageJobSettled(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "settled")
	apiKeyID := int64(2001)
	accountID := int64(3001)
	providerJob := "providers/job"
	outputRef := "files/output"
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:           batchID,
		UserID:            1001,
		APIKeyID:          &apiKeyID,
		AccountID:         &accountID,
		Provider:          service.BatchImageProviderGeminiAPI,
		Model:             "gemini-image",
		Status:            service.BatchImageJobStatusSettling,
		ProviderJobName:   &providerJob,
		ProviderOutputRef: &outputRef,
		ItemCount:         3,
		SuccessCount:      2,
		FailCount:         1,
	})
	require.NoError(t, err)

	err = repo.MarkBatchImageJobSettled(ctx, service.MarkBatchImageJobSettledParams{
		BatchID:      batchID,
		ActualCost:   0.5,
		ManifestHash: "manifest-hash",
		EventPayload: map[string]any{"request_id": "batch_image_settlement:" + batchID},
		Now:          &now,
	})
	require.NoError(t, err)

	job, err := repo.GetBatchImageJobByBatchID(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, service.BatchImageJobStatusCompleted, job.Status)
	require.NotNil(t, job.ActualCost)
	require.Equal(t, 0.5, *job.ActualCost)
	require.Equal(t, "manifest-hash", batchImageDerefTest(job.ManifestHash))
	require.NotNil(t, job.SettledAt)
	require.Equal(t, now, *job.SettledAt)

	var eventCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM batch_image_events WHERE job_id = $1 AND event_type = 'settlement_completed'`, batchID).Scan(&eventCount)
	require.NoError(t, err)
	require.Equal(t, 1, eventCount)
}

func TestBatchImageRepository_SetBatchImageJobSettlementFailed(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "settlement-failed")

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:      batchID,
		UserID:       1001,
		Provider:     service.BatchImageProviderGeminiAPI,
		Model:        "gemini-image",
		Status:       service.BatchImageJobStatusSettling,
		ItemCount:    1,
		SuccessCount: 1,
	})
	require.NoError(t, err)

	retryCount, err := repo.SetBatchImageJobSettlementFailed(ctx, batchID, "SETTLEMENT_BILLING_FAILED", "temporary")
	require.NoError(t, err)
	require.Equal(t, 1, retryCount)

	job, err := repo.GetBatchImageJobByBatchID(ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, service.BatchImageJobStatusSettling, job.Status)
	require.Equal(t, "SETTLEMENT_BILLING_FAILED", batchImageDerefTest(job.LastErrorCode))
	require.Equal(t, "temporary", batchImageDerefTest(job.LastErrorMessage))
	require.Equal(t, 1, job.RetryCount)
}

func TestBatchImageRepository_AppendEvent(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	repo := newBatchImageRepositoryWithSQL(tx)
	batchID := batchImageTestID(t, "event")

	_, err := repo.CreateBatchImageJob(ctx, service.CreateBatchImageJobParams{
		BatchID:   batchID,
		UserID:    1001,
		Provider:  service.BatchImageProviderVertex,
		Model:     "gemini-2.5-flash-image",
		ItemCount: 1,
	})
	require.NoError(t, err)

	err = repo.AppendBatchImageEvent(ctx, batchID, "job_created", map[string]any{"batch_id": batchID})
	require.NoError(t, err)

	var payload string
	err = tx.QueryRowContext(ctx, `SELECT payload::text FROM batch_image_events WHERE job_id = $1 AND event_type = 'job_created'`, batchID).Scan(&payload)
	require.NoError(t, err)
	require.Contains(t, payload, batchID)
}

func createBatchImageTestProject(t *testing.T, tx batchImageSQLExecutor, slug string) int64 {
	t.Helper()
	var projectID int64
	err := scanSingleRow(context.Background(), tx, `
		INSERT INTO projects (name, slug, description, status, profiles, created_at, updated_at)
		VALUES ($1, $2, $3, $4, '{}'::jsonb, NOW(), NOW())
		RETURNING id
	`, []any{slug, slug, "Batch image repository integration project.", service.StatusActive}, &projectID)
	require.NoError(t, err)
	return projectID
}

func batchImageTestJobIDs(jobs []*service.BatchImageJob) []string {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.BatchID)
	}
	return ids
}

func batchImageTestID(t *testing.T, prefix string) string {
	t.Helper()
	safePrefix := batchImageSafeTestIDSegment(prefix, 20)
	sum := sha1.Sum([]byte(t.Name()))
	return "imgbatch_" + safePrefix + "_" + hex.EncodeToString(sum[:])[:16]
}

func batchImageSafeTestIDSegment(v string, maxLen int) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(v, "-")
	v = strings.Trim(v, "-_")
	if v == "" {
		v = "job"
	}
	if len(v) > maxLen {
		v = v[:maxLen]
		v = strings.Trim(v, "-_")
	}
	if v == "" {
		return "job"
	}
	return v
}

func batchImageTestStringPtr(v string) *string {
	return &v
}

func batchImageDerefTest(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
