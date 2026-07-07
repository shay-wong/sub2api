package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultBatchImageBillingRecoveryStaleAfter = 10 * time.Minute
	defaultBatchImageBillingRecoveryLimit      = 100
)

type BatchImageBillingRecoveryService struct {
	Repo       BatchImageRepository
	Billing    UsageBillingRepository
	AuthCache  APIKeyAuthCacheInvalidator
	Queue      BatchImageQueue
	StaleAfter time.Duration
	Limit      int
}

func (s *BatchImageBillingRecoveryService) ReleaseStaleUnsubmittedOnce(ctx context.Context) (int, error) {
	if s == nil || s.Repo == nil || s.Billing == nil {
		return 0, nil
	}
	staleAfter := s.StaleAfter
	if staleAfter <= 0 {
		staleAfter = defaultBatchImageBillingRecoveryStaleAfter
	}
	limit := s.Limit
	if limit <= 0 {
		limit = defaultBatchImageBillingRecoveryLimit
	}
	cutoff := time.Now().Add(-staleAfter)
	jobs, err := s.Repo.ListStaleUnsubmittedBatchImageJobs(ctx, cutoff, limit)
	if err != nil {
		return 0, err
	}
	released := 0
	var lastErr error
	for _, job := range jobs {
		if job == nil {
			continue
		}
		jobCtx := batchImageContextForJob(ctx, job)
		if err := jobCtx.Err(); err != nil {
			return released, err
		}
		retryRelease := isBatchImageBillingReleaseRetryJob(job)
		msg := "batch image submission did not reach provider before recovery cutoff"
		// 原子转 failed 并复核 stale 条件：List 与转态之间 job 可能已被慢提交
		// 心跳续期或提交成功（provider_job_name 已写入），此时绝不能退款，
		// 否则上游任务照常产生成本而用户已拿回冻结余额。
		applied := retryRelease
		if !retryRelease {
			var err error
			applied, err = s.Repo.FailStaleUnsubmittedBatchImageJob(jobCtx, job.BatchID, cutoff, BatchImageErrorSubmitStaleBeforeProvider, msg)
			if err != nil {
				// applied=true 时 UPDATE 已提交（仅审计事件写入失败）：必须继续释放，
				// 否则 job 已转 failed、不再出现在 stale 列表，冻结余额会永久泄漏。
				if !applied {
					lastErr = err
					continue
				}
				logger.L().Warn("batch_image.recovery_fail_event_append_failed",
					zap.String("batch_id", job.BatchID),
					zap.Error(err),
				)
			}
		}
		if !applied {
			continue
		}
		if !retryRelease {
			job.Status = BatchImageJobStatusFailed
			job.LastErrorCode = batchImageStringPtr(BatchImageErrorSubmitStaleBeforeProvider)
			job.LastErrorMessage = batchImageStringPtr(msg)
		}
		if err := releaseBatchImageHoldWithRecovery(jobCtx, s.Repo, s.Billing, s.Queue, s.AuthCache, job, batchImageDerefString(job.RequestHash)); err != nil {
			// 释放失败必须持久化为可扫描状态；如果 Redis 入队也失败，
			// 下一轮 recovery 仍能从 DB 重新捡起这个 failed job。
			logger.L().Warn("batch_image.recovery_release_failed",
				zap.String("batch_id", job.BatchID),
				zap.Error(err),
			)
			lastErr = err
			continue
		}
		released++
	}
	return released, lastErr
}

func isBatchImageBillingReleaseRetryJob(job *BatchImageJob) bool {
	return job != nil &&
		(job.Status == BatchImageJobStatusFailed || job.Status == BatchImageJobStatusCancelled) &&
		isBatchImageBillingReleaseRecoveryMarker(batchImageDerefString(job.LastErrorCode))
}

func isBatchImageBillingReleaseRecoveryMarker(code string) bool {
	return code == BatchImageErrorBillingReleaseFailed || code == BatchImageErrorSubmitStaleBeforeProvider
}
