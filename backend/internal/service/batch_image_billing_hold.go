package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	batchImageHoldRequestPrefix    = "batch_image_hold:"
	batchImageCaptureRequestPrefix = "batch_image_capture:"
	batchImageReleaseRequestPrefix = "batch_image_release:"
)

func BatchImageHoldRequestID(batchID string) string {
	return batchImageHoldRequestPrefix + strings.TrimSpace(batchID)
}

func BatchImageCaptureRequestID(batchID string) string {
	return batchImageCaptureRequestPrefix + strings.TrimSpace(batchID)
}

func BatchImageReleaseRequestID(batchID string) string {
	return batchImageReleaseRequestPrefix + strings.TrimSpace(batchID)
}

func buildBatchImageHoldCommand(job *BatchImageJob, requestID string, actualAmount float64, payloadHash string) (*BatchImageBalanceHoldCommand, error) {
	if job == nil {
		return nil, ErrBatchImageBillingHoldFailed
	}
	if job.APIKeyID == nil || *job.APIKeyID <= 0 {
		return nil, ErrBatchImageSettlementMissingAPIKeyID
	}
	holdAmount := job.EstimatedCost
	if job.HoldAmount != nil {
		holdAmount = *job.HoldAmount
	}
	if holdAmount < 0 {
		holdAmount = 0
	}
	if actualAmount < 0 {
		actualAmount = 0
	}
	return &BatchImageBalanceHoldCommand{
		RequestID:          requestID,
		APIKeyID:           *job.APIKeyID,
		UserID:             job.UserID,
		BatchID:            job.BatchID,
		HoldAmount:         holdAmount,
		ActualAmount:       actualAmount,
		RequestPayloadHash: strings.TrimSpace(payloadHash),
	}, nil
}

func reserveBatchImageBalanceHold(ctx context.Context, repo UsageBillingRepository, job *BatchImageJob, payloadHash string) error {
	if repo == nil {
		return ErrBatchImageBillingHoldFailed.WithCause(errors.New("batch image billing repository is not configured"))
	}
	cmd, err := buildBatchImageHoldCommand(job, BatchImageHoldRequestID(job.BatchID), 0, payloadHash)
	if err != nil {
		return err
	}
	if cmd.HoldAmount <= 0 {
		return nil
	}
	if _, err := repo.ReserveBatchImageBalance(ctx, cmd); err != nil {
		if errors.Is(err, ErrBatchImageInsufficientBalance) {
			return ErrBatchImageInsufficientBalance
		}
		return ErrBatchImageBillingHoldFailed.WithCause(err)
	}
	return nil
}

func captureBatchImageBalanceHold(ctx context.Context, repo UsageBillingRepository, job *BatchImageJob, actualAmount float64, payloadHash string) error {
	if repo == nil {
		return ErrBatchImageSettlementBillingFailed.WithCause(errors.New("batch image billing repository is not configured"))
	}
	cmd, err := buildBatchImageHoldCommand(job, BatchImageCaptureRequestID(job.BatchID), actualAmount, payloadHash)
	if err != nil {
		return err
	}
	if _, err := repo.CaptureBatchImageBalance(ctx, cmd); err != nil {
		return ErrBatchImageSettlementBillingFailed.WithCause(err)
	}
	return nil
}

func releaseBatchImageBalanceHold(ctx context.Context, repo UsageBillingRepository, job *BatchImageJob, payloadHash string) error {
	if repo == nil || job == nil {
		return nil
	}
	cmd, err := buildBatchImageHoldCommand(job, BatchImageReleaseRequestID(job.BatchID), 0, payloadHash)
	if err != nil {
		return err
	}
	if cmd.HoldAmount <= 0 {
		return nil
	}
	if _, err := repo.ReleaseBatchImageBalance(ctx, cmd); err != nil {
		// 同一 release request id 出现指纹冲突，说明此前已有一次携带不同
		// payloadHash 的释放成功提交（资金已归还）。视为幂等成功，
		// 避免历史指纹不一致的 job 永远卡在释放失败的毒消息循环里。
		if errors.Is(err, ErrUsageBillingRequestConflict) {
			logger.L().Warn("batch_image.release_fingerprint_conflict_treated_as_released",
				zap.String("batch_id", job.BatchID),
			)
			return nil
		}
		return ErrBatchImageBillingHoldFailed.WithCause(err)
	}
	return nil
}

func releaseBatchImageHoldWithRecovery(
	ctx context.Context,
	repo BatchImageRepository,
	billing UsageBillingRepository,
	queue BatchImageQueue,
	authCache APIKeyAuthCacheInvalidator,
	job *BatchImageJob,
	payloadHash string,
) error {
	if job == nil {
		return nil
	}
	releaseCtx, releaseCancel := batchImageCompensationContext(ctx)
	releaseErr := releaseBatchImageBalanceHold(releaseCtx, billing, job, payloadHash)
	releaseCancel()
	if releaseErr != nil {
		stateCtx, stateCancel := batchImageCompensationContext(ctx)
		defer stateCancel()
		var markErr error
		if repo != nil {
			markErr = repo.MarkBatchImageBillingReleaseFailed(stateCtx, job.BatchID, sanitizeBatchImagePublicMessage(releaseErr.Error()))
		}
		if markErr != nil {
			return ErrBatchImageBillingHoldFailed.WithCause(errors.Join(releaseErr, markErr))
		}
		enqueueErr := enqueueBatchImageBillingRetry(stateCtx, repo, queue, job.BatchID)
		if enqueueErr != nil {
			return ErrBatchImageBillingHoldFailed.WithCause(errors.Join(releaseErr, enqueueErr))
		}
		return ErrBatchImageBillingHoldFailed.WithCause(releaseErr)
	}

	stateCtx, stateCancel := batchImageCompensationContext(ctx)
	defer stateCancel()
	if repo != nil && isBatchImageBillingReleaseRecoveryMarker(batchImageDerefString(job.LastErrorCode)) {
		if err := repo.MarkBatchImageBillingReleaseRecovered(stateCtx, job.BatchID); err != nil {
			return err
		}
		job.LastErrorCode = batchImageStringPtr(BatchImageErrorBillingReleaseRecovered)
		job.LastErrorMessage = batchImageStringPtr("billing hold released after retry")
	}
	if authCache != nil && job.UserID > 0 {
		authCache.InvalidateAuthCacheByUserID(stateCtx, job.UserID)
	}
	return nil
}

func enqueueBatchImageBillingRetry(ctx context.Context, repo BatchImageRepository, queue BatchImageQueue, batchID string) error {
	if queue == nil {
		return nil
	}
	if err := queue.Enqueue(ctx, batchID); err != nil && !errors.Is(err, ErrBatchImageAlreadyQueued) {
		logger.L().Warn("batch_image.billing_retry_enqueue_failed",
			zap.String("batch_id", batchID),
			zap.Error(err),
		)
		if repo != nil {
			if eventErr := repo.AppendBatchImageEvent(ctx, batchID, "billing_retry_enqueue_failed", map[string]any{
				"batch_id": batchID,
				"error":    sanitizeBatchImagePublicMessage(err.Error()),
			}); eventErr != nil {
				logger.L().Warn("batch_image.billing_retry_event_failed",
					zap.String("batch_id", batchID),
					zap.Error(eventErr),
				)
			}
		}
		return err
	}
	return nil
}
